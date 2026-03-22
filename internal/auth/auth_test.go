package auth

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- HandleAuthorize tests ---

// Client-supplied scope must be overridden with the required openid scopes.
// Without this, a client could request narrower scopes and break the Dex flow.
func TestHandleAuthorize_OverridesScope(t *testing.T) {
	handler := HandleAuthorize("https://argocd.example.com/api/dex/auth", "argo-cd-cli")
	req := httptest.NewRequest(http.MethodGet, "/authorize?scope=custom&state=abc", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if strings.Contains(location, "scope=custom") {
		t.Error("expected custom scope to be overridden")
	}
	// Verify all required scopes are present.
	for _, scope := range []string{"openid", "profile", "email", "groups", "offline_access"} {
		if !strings.Contains(location, scope) {
			t.Errorf("expected scope %q in redirect, got: %s", scope, location)
		}
	}
	// State must be forwarded for CSRF protection.
	if !strings.Contains(location, "state=abc") {
		t.Errorf("expected state forwarded, got: %s", location)
	}
	// client_id must be injected.
	if !strings.Contains(location, "client_id=argo-cd-cli") {
		t.Errorf("expected client_id injected, got: %s", location)
	}
}

// Invalid dexAuthURL must return 500, not panic.
func TestHandleAuthorize_InvalidDexURL(t *testing.T) {
	handler := HandleAuthorize("://invalid", "argo-cd-cli")
	req := httptest.NewRequest(http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for invalid dex URL, got %d", rec.Code)
	}
}

// --- HandleRegister tests ---

// Empty body must not crash — MCP clients may send minimal requests.
func TestHandleRegister_EmptyBody(t *testing.T) {
	handler := HandleRegister("argo-cd-cli")
	req := httptest.NewRequest(http.MethodPost, "/register", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 even with empty body, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["client_id"] != "argo-cd-cli" {
		t.Errorf("expected client_id=argo-cd-cli, got %v", resp["client_id"])
	}
	if resp["token_endpoint_auth_method"] != "none" {
		t.Errorf("expected auth_method=none (public client), got %v", resp["token_endpoint_auth_method"])
	}
}

// Malformed JSON body must not crash — returns client_id with zeroed optional fields.
func TestHandleRegister_MalformedBody(t *testing.T) {
	handler := HandleRegister("argo-cd-cli")
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"redirect_uris": "not-an-array"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for malformed body, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["client_id"] != "argo-cd-cli" {
		t.Errorf("expected client_id, got %v", resp["client_id"])
	}
	// Optional fields should be absent when decode fails.
	if _, ok := resp["redirect_uris"]; ok {
		t.Error("expected redirect_uris to be absent for malformed input")
	}
}

// --- HandleToken tests ---

// Core behavior: id_token must be swapped into access_token because ArgoCD
// validates the id_token JWT, not the opaque access_token from Dex.
func TestHandleToken_SwapsIdTokenToAccessToken(t *testing.T) {
	dex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "opaque-access",
			"id_token":     "jwt-id-token",
			"token_type":   "bearer",
		})
	}))
	defer dex.Close()

	handler := HandleToken(dex.URL, "argo-cd-cli")
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader("grant_type=authorization_code&code=abc"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode token response: %v", err)
	}
	if resp["access_token"] != "jwt-id-token" {
		t.Errorf("expected access_token=jwt-id-token (swapped), got %v", resp["access_token"])
	}
	if resp["token_type"] != "Bearer" {
		t.Errorf("expected token_type=Bearer, got %v", resp["token_type"])
	}
}

// When Dex returns no id_token (e.g. client_credentials flow), the swap must NOT
// happen — access_token should be left as-is.
func TestHandleToken_NoIdToken_NoSwap(t *testing.T) {
	dex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "original-access",
			"token_type":   "bearer",
		})
	}))
	defer dex.Close()

	handler := HandleToken(dex.URL, "argo-cd-cli")
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader("grant_type=authorization_code&code=abc"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode token response: %v", err)
	}
	if resp["access_token"] != "original-access" {
		t.Errorf("expected access_token unchanged, got %v", resp["access_token"])
	}
}

// Dex returns invalid JSON — handler must forward raw body instead of crashing.
func TestHandleToken_InvalidJSON_ForwardsRawBody(t *testing.T) {
	dex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer dex.Close()

	handler := HandleToken(dex.URL, "argo-cd-cli")
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader("grant_type=authorization_code&code=abc"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not json") {
		t.Errorf("expected raw body forwarded, got: %s", rec.Body.String())
	}
}

// client_id must be injected, overriding whatever the client sent (prevents spoofing).
func TestHandleToken_ForwardsClientID(t *testing.T) {
	var receivedClientID string
	dex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		receivedClientID = r.FormValue("client_id")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok"})
	}))
	defer dex.Close()

	handler := HandleToken(dex.URL, "my-client")
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader("grant_type=authorization_code&code=abc&client_id=attacker"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if receivedClientID != "my-client" {
		t.Errorf("expected client_id=my-client (overridden), got %q", receivedClientID)
	}
}

// Dex returns an error — status code AND body must be forwarded to the client.
func TestHandleToken_ForwardsDexError(t *testing.T) {
	dex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer dex.Close()

	handler := HandleToken(dex.URL, "argo-cd-cli")
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader("grant_type=authorization_code&code=bad"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 forwarded from Dex, got %d", rec.Code)
	}
	// Body must also be forwarded.
	if !strings.Contains(rec.Body.String(), "invalid_grant") {
		t.Errorf("expected Dex error body forwarded, got: %s", rec.Body.String())
	}
}

// Dex is down — must return 502, not crash or hang.
func TestHandleToken_DexUnreachable(t *testing.T) {
	// Use a closed server to guarantee connection refused.
	dex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dexURL := dex.URL
	dex.Close()

	handler := HandleToken(dexURL, "argo-cd-cli")
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader("grant_type=authorization_code&code=abc"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

// --- Metadata tests ---

// Auth server metadata must return all fields required by the MCP OAuth spec.
func TestHandleAuthServerMetadata_ReturnsValidDocument(t *testing.T) {
	handler := HandleAuthServerMetadata("http://localhost:8080")
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var meta map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&meta); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	required := map[string]string{
		"issuer":                 "http://localhost:8080",
		"authorization_endpoint": "http://localhost:8080/authorize",
		"token_endpoint":         "http://localhost:8080/token",
		"registration_endpoint":  "http://localhost:8080/register",
	}
	for field, expected := range required {
		if meta[field] != expected {
			t.Errorf("expected %s=%q, got %v", field, expected, meta[field])
		}
	}
	// PKCE support is required for public clients.
	methods, ok := meta["code_challenge_methods_supported"].([]any)
	if !ok || len(methods) == 0 {
		t.Error("expected code_challenge_methods_supported to be present")
	}
}

// Protected resource metadata is what MCP clients use to discover the OAuth flow.
func TestHandleProtectedResourceMetadata_ReturnsValidDocument(t *testing.T) {
	handler := HandleProtectedResourceMetadata("http://localhost:8080")
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var meta map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&meta); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if meta["resource"] != "http://localhost:8080" {
		t.Errorf("expected resource=http://localhost:8080, got %v", meta["resource"])
	}
	servers, ok := meta["authorization_servers"].([]any)
	if !ok || len(servers) == 0 || servers[0] != "http://localhost:8080" {
		t.Errorf("expected authorization_servers=[http://localhost:8080], got %v", meta["authorization_servers"])
	}
}

// --- Middleware tests ---

// Valid bearer token must be stored in context and passed to the inner handler.
func TestMiddleware_ValidBearer_PassesThrough(t *testing.T) {
	middleware := NewPassthroughMiddleware("http://localhost:8080", silentLogger())

	var capturedToken string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, _ := GetBearerToken(r.Context())
		capturedToken = tok
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(inner)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer my-jwt-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if capturedToken != "my-jwt-token" {
		t.Errorf("expected token=my-jwt-token in context, got %q", capturedToken)
	}
}

// RFC 6750: "bearer" is case-insensitive.
func TestMiddleware_LowercaseBearer_Accepted(t *testing.T) {
	middleware := NewPassthroughMiddleware("http://localhost:8080", silentLogger())

	var capturedToken string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, _ := GetBearerToken(r.Context())
		capturedToken = tok
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(inner)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "bearer my-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if capturedToken != "my-token" {
		t.Errorf("expected case-insensitive bearer accepted, got %q", capturedToken)
	}
}

// No Authorization header must return 401 with WWW-Authenticate pointing to
// the correct OAuth discovery URL — this is how MCP clients discover the OAuth flow.
func TestMiddleware_MissingAuth_Returns401WithDiscovery(t *testing.T) {
	middleware := NewPassthroughMiddleware("http://mcp.example.com", silentLogger())
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})

	handler := middleware(inner)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	wwwAuth := rec.Header().Get("WWW-Authenticate")
	expected := `Bearer resource_metadata="http://mcp.example.com/.well-known/oauth-protected-resource"`
	if wwwAuth != expected {
		t.Errorf("expected WWW-Authenticate=%q, got %q", expected, wwwAuth)
	}
}

// "Bearer " (empty token) must be rejected.
func TestMiddleware_EmptyBearer_Returns401(t *testing.T) {
	middleware := NewPassthroughMiddleware("http://localhost:8080", silentLogger())
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})

	handler := middleware(inner)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for empty bearer, got %d", rec.Code)
	}
}

// Basic auth must be rejected — only Bearer is supported.
func TestMiddleware_BasicAuth_Rejected(t *testing.T) {
	middleware := NewPassthroughMiddleware("http://localhost:8080", silentLogger())
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})

	handler := middleware(inner)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for Basic auth, got %d", rec.Code)
	}
}

// --- ParseTokenClaims tests ---

// Valid JWT must extract claims correctly — used for audit logging.
func TestParseTokenClaims_ValidJWT(t *testing.T) {
	// Payload: {"sub":"user1","email":"alice@example.com","name":"Alice","groups":["admin"]}
	token := "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyMSIsImVtYWlsIjoiYWxpY2VAZXhhbXBsZS5jb20iLCJuYW1lIjoiQWxpY2UiLCJncm91cHMiOlsiYWRtaW4iXX0.signature"
	claims := ParseTokenClaims(token)
	if claims == nil {
		t.Fatal("expected claims, got nil")
	}
	if claims.Sub != "user1" {
		t.Errorf("expected sub=user1, got %q", claims.Sub)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("expected email=alice@example.com, got %q", claims.Email)
	}
	if claims.Name != "Alice" {
		t.Errorf("expected name=Alice, got %q", claims.Name)
	}
	if len(claims.Groups) != 1 || claims.Groups[0] != "admin" {
		t.Errorf("expected groups=[admin], got %v", claims.Groups)
	}
}

// Invalid tokens must return nil without panicking.
func TestParseTokenClaims_InvalidInputs(t *testing.T) {
	cases := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"not a JWT", "not-a-jwt"},
		{"invalid base64", "a.!!!.c"},
		{"two parts only", "a.b"},
		{"valid base64 but invalid JSON", "a.aGVsbG8.c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if ParseTokenClaims(tc.token) != nil {
				t.Errorf("expected nil for %q", tc.token)
			}
		})
	}
}

// --- Context helpers tests ---

// Empty string stored in context must be treated as "no token" —
// prevents empty Bearer tokens from propagating.
func TestGetBearerToken_EmptyStringIsNoToken(t *testing.T) {
	ctx := WithBearerToken(context.Background(), "")
	_, ok := GetBearerToken(ctx)
	if ok {
		t.Error("expected empty string to be treated as no token")
	}
}
