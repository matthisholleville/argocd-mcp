package auth

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Providers bake parameters into the authorization_endpoint they advertise
// (Azure AD B2C's ?p=<policy>); overwriting the query breaks every login.
func TestHandleAuthorize_PreservesEndpointQuery(t *testing.T) {
	h := HandleAuthorize(AuthorizeOptions{
		AuthorizationEndpoint: "https://t.b2clogin.com/t/oauth2/v2.0/authorize?p=B2C_1_signin",
		ClientID:              "cid",
		Scopes:                "openid",
	})

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/authorize?response_type=code", nil))

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("unparseable Location: %v", err)
	}
	if got := loc.Query().Get("p"); got != "B2C_1_signin" {
		t.Errorf("provider's own endpoint parameter was dropped: p=%q", got)
	}
	if got := loc.Query().Get("client_id"); got != "cid" {
		t.Errorf("client_id not applied: %q", got)
	}
}

// A client's redirect_uri may carry a parameter it needs to correlate the
// callback with the request it started.
func TestHandleCallback_PreservesClientRedirectQuery(t *testing.T) {
	codec, err := NewStateCodec()
	if err != nil {
		t.Fatalf("NewStateCodec: %v", err)
	}
	state, err := codec.Encode("http://127.0.0.1:53421/callback?session=abc123", "clientstate")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	rec := httptest.NewRecorder()
	h := HandleCallback(codec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h(rec, httptest.NewRequest(http.MethodGet, "/oauth/callback?code=c&state="+url.QueryEscape(state), nil))

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("unparseable Location: %v", err)
	}
	q := loc.Query()
	if q.Get("session") != "abc123" {
		t.Errorf("client's own redirect parameter was dropped: session=%q", q.Get("session"))
	}
	if q.Get("code") != "c" || q.Get("state") != "clientstate" {
		t.Errorf("unexpected relayed params: %v", q)
	}
}

// To the MCP client the authorization server is this server, so relaying the
// upstream issuer fails an RFC 9207 check on the client side.
func TestHandleCallback_DoesNotRelayUpstreamIss(t *testing.T) {
	codec, err := NewStateCodec()
	if err != nil {
		t.Fatalf("NewStateCodec: %v", err)
	}
	state, err := codec.Encode("http://127.0.0.1:53421/callback", "")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	rec := httptest.NewRecorder()
	h := HandleCallback(codec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h(rec, httptest.NewRequest(http.MethodGet,
		"/oauth/callback?code=c&iss=https%3A%2F%2Fkeycloak.example.com%2Frealms%2Fx&state="+url.QueryEscape(state), nil))

	loc, _ := url.Parse(rec.Header().Get("Location"))
	if got := loc.Query().Get("iss"); got != "" {
		t.Errorf("upstream iss was relayed to the client: %q", got)
	}
}

// The proxy's contract is the id_token-for-access_token swap. If the provider
// returned no id_token, handing the client the opaque access_token produces a
// login that looks successful and then 401s on every ArgoCD call.
func TestHandleToken_RequireIDToken_FailsWithoutIDToken(t *testing.T) {
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"OPAQUE-not-a-jwt","token_type":"Bearer","expires_in":3600}`))
	}))
	defer idp.Close()

	rec := httptest.NewRecorder()
	h := HandleToken(TokenOptions{TokenEndpoint: idp.URL, ClientID: "cid", RequireIDToken: true})
	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader("grant_type=authorization_code&code=c"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when the provider returns no id_token, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if !strings.Contains(body["error_description"], "openid") {
		t.Errorf("error should point at the missing openid scope, got %q", body["error_description"])
	}
}

// Discovery must refuse a document that declares a different issuer, the check
// that stops a redirect or a tampered issuer swapping in another origin's
// endpoints.
func TestDiscoverProvider_RejectsIssuerMismatch(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"issuer": "https://attacker.example.com",
			"authorization_endpoint": "https://attacker.example.com/authorize",
			"token_endpoint": "https://attacker.example.com/token"
		}`))
	}))
	defer provider.Close()

	if _, err := DiscoverProvider(t.Context(), testClient(), provider.URL); err == nil {
		t.Fatal("expected an issuer mismatch to be rejected")
	}
}
