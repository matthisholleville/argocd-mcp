package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// --- Provider-agnostic authorize tests (oidc mode) ---

// Scopes are per-provider: Dex wants "groups offline_access", others reject
// unknown scopes outright, so what the proxy sends must be what was configured.
func TestHandleAuthorize_UsesConfiguredScopes(t *testing.T) {
	handler := HandleAuthorize(AuthorizeOptions{
		AuthorizationEndpoint: "https://idp.example.com/oauth2/v1/authorize",
		ClientID:              "okta-client",
		Scopes:                "openid email",
	})
	req := httptest.NewRequest(http.MethodGet, "/authorize?scope=openid+profile+email+groups+offline_access&state=xyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "scope=openid+email") {
		t.Errorf("expected the configured scopes, got: %s", location)
	}
	if strings.Contains(location, "offline_access") {
		t.Errorf("expected the client scopes to be replaced, got: %s", location)
	}
	if !strings.Contains(location, "client_id=okta-client") {
		t.Errorf("expected client_id injected, got: %s", location)
	}
	if !strings.Contains(location, "state=xyz") {
		t.Errorf("expected state forwarded, got: %s", location)
	}
}

// ArgoCD RBAC keys off the groups claim, which some providers only emit when
// the flow asks for it explicitly.
func TestHandleAuthorize_ForwardsRequestedIDTokenClaims(t *testing.T) {
	claims := `{"id_token":{"groups":{"essential":true}}}`
	handler := HandleAuthorize(AuthorizeOptions{
		AuthorizationEndpoint:  "https://idp.example.com/oauth2/v1/authorize",
		ClientID:               "okta-client",
		Scopes:                 "openid",
		RequestedIDTokenClaims: claims,
	})
	req := httptest.NewRequest(http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("could not parse redirect: %v", err)
	}
	if got := location.Query().Get("claims"); got != claims {
		t.Errorf("expected claims %q, got %q", claims, got)
	}
}

// No claims configured must mean no claims param at all, rather than an empty
// one that a provider may reject as malformed.
func TestHandleAuthorize_OmitsEmptyClaims(t *testing.T) {
	handler := HandleAuthorize(AuthorizeOptions{
		AuthorizationEndpoint: "https://idp.example.com/oauth2/v1/authorize",
		ClientID:              "okta-client",
		Scopes:                "openid",
	})
	req := httptest.NewRequest(http.MethodGet, "/authorize", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if strings.Contains(rec.Header().Get("Location"), "claims=") {
		t.Errorf("expected no claims param, got: %s", rec.Header().Get("Location"))
	}
}

// --- Provider-agnostic token tests (oidc mode) ---

// Confidential clients need the secret on the exchange, and it must be added
// server-side so MCP clients stay public.
func TestHandleToken_SendsClientSecretWhenConfigured(t *testing.T) {
	var receivedSecret string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		receivedSecret = r.FormValue("client_secret")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id_token": "jwt", "access_token": "opaque"})
	}))
	defer idp.Close()

	handler := HandleToken(TokenOptions{TokenEndpoint: idp.URL, ClientID: "okta-client", ClientSecret: "s3cret"})
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader("grant_type=authorization_code&code=abc"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if receivedSecret != "s3cret" {
		t.Errorf("expected the client secret forwarded to the provider, got %q", receivedSecret)
	}
}

// Public clients (Dex's argo-cd-cli) rely on PKCE alone, and an empty
// client_secret is enough for some providers to reject the exchange.
func TestHandleToken_OmitsClientSecretWhenUnset(t *testing.T) {
	var hasSecret bool
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		_, hasSecret = r.Form["client_secret"]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id_token": "jwt"})
	}))
	defer idp.Close()

	handler := HandleToken(TokenOptions{TokenEndpoint: idp.URL, ClientID: "argo-cd-cli"})
	req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader("grant_type=authorization_code&code=abc"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if hasSecret {
		t.Error("expected no client_secret for a public client")
	}
}
