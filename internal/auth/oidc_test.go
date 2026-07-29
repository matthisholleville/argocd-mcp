package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient() *http.Client { return &http.Client{} }

// issuerOf rebuilds the URL the test server is reachable at, so fixtures can
// declare the issuer the discovery document is required to match.
func issuerOf(r *http.Request) string { return "http://" + r.Host }

// --- DiscoverProvider tests ---

func TestDiscoverProvider_ReadsEndpoints(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			t.Errorf("unexpected discovery path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"issuer": "` + issuerOf(r) + `",
			"authorization_endpoint": "https://idp.example.com/oauth2/v1/authorize",
			"token_endpoint": "https://idp.example.com/oauth2/v1/token"
		}`))
	}))
	defer provider.Close()

	endpoints, err := DiscoverProvider(context.Background(), testClient(), provider.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if endpoints.AuthorizationEndpoint != "https://idp.example.com/oauth2/v1/authorize" {
		t.Errorf("unexpected authorization endpoint: %q", endpoints.AuthorizationEndpoint)
	}
	if endpoints.TokenEndpoint != "https://idp.example.com/oauth2/v1/token" {
		t.Errorf("unexpected token endpoint: %q", endpoints.TokenEndpoint)
	}
}

// A trailing slash on the issuer must not produce a double slash in the
// discovery URL, which some providers 404 on.
func TestDiscoverProvider_TrimsTrailingSlash(t *testing.T) {
	var gotPath string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"issuer":"` + issuerOf(r) + `","authorization_endpoint":"https://a","token_endpoint":"https://t"}`))
	}))
	defer provider.Close()

	if _, err := DiscoverProvider(context.Background(), testClient(), provider.URL+"/"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/.well-known/openid-configuration" {
		t.Errorf("expected single-slash discovery path, got %q", gotPath)
	}
}

// A document missing an endpoint must fail at startup rather than yield a proxy
// that redirects nowhere.
func TestDiscoverProvider_IncompleteDocument(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"authorization_endpoint": "https://idp.example.com/authorize"}`))
	}))
	defer provider.Close()

	_, err := DiscoverProvider(context.Background(), testClient(), provider.URL)
	if err == nil {
		t.Fatal("expected an error for a document without token_endpoint")
	}
	if !strings.Contains(err.Error(), "token_endpoint") {
		t.Errorf("expected the error to name the missing field, got: %v", err)
	}
}

func TestDiscoverProvider_NonOKStatus(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer provider.Close()

	if _, err := DiscoverProvider(context.Background(), testClient(), provider.URL); err == nil {
		t.Fatal("expected an error for a 404 discovery document")
	}
}

// --- DiscoverArgoCDOIDC tests ---

func TestDiscoverArgoCDOIDC_ReadsIssuerAndClientID(t *testing.T) {
	argocd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/settings" {
			t.Errorf("unexpected settings path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"oidcConfig":{"name":"Okta","issuer":"https://idp.example.com","clientID":"web-app"}}`))
	}))
	defer argocd.Close()

	settings, err := DiscoverArgoCDOIDC(context.Background(), testClient(), argocd.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.Issuer != "https://idp.example.com" {
		t.Errorf("unexpected issuer: %q", settings.Issuer)
	}
	if settings.ClientID != "web-app" {
		t.Errorf("unexpected client id: %q", settings.ClientID)
	}
}

// cliClientID is the client ArgoCD intends for non-browser callers, so it wins
// over clientID when both are declared.
func TestDiscoverArgoCDOIDC_PrefersCLIClientID(t *testing.T) {
	argocd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"oidcConfig":{"issuer":"https://idp.example.com","clientID":"web-app","cliClientID":"cli-app"}}`))
	}))
	defer argocd.Close()

	settings, err := DiscoverArgoCDOIDC(context.Background(), testClient(), argocd.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.ClientID != "cli-app" {
		t.Errorf("expected cliClientID to win, got %q", settings.ClientID)
	}
}

// A Dex-backed ArgoCD would reject tokens from an external provider, so the
// error has to point at the mode that does work rather than fail later at login.
func TestDiscoverArgoCDOIDC_DexBackedArgoCD(t *testing.T) {
	argocd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"dexConfig":{"connectors":[{"name":"Okta","type":"saml"}]}}`))
	}))
	defer argocd.Close()

	_, err := DiscoverArgoCDOIDC(context.Background(), testClient(), argocd.URL)
	if err == nil {
		t.Fatal("expected an error against a Dex-backed ArgoCD")
	}
	if !strings.Contains(err.Error(), "AUTH_MODE=oauth") {
		t.Errorf("expected the error to point at oauth mode, got: %v", err)
	}
}

func TestDiscoverArgoCDOIDC_NoAuthConfigured(t *testing.T) {
	argocd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://argocd.example.com"}`))
	}))
	defer argocd.Close()

	_, err := DiscoverArgoCDOIDC(context.Background(), testClient(), argocd.URL)
	if err == nil {
		t.Fatal("expected an error when ArgoCD advertises no OIDC config")
	}
	if !strings.Contains(err.Error(), "OIDC_ISSUER") {
		t.Errorf("expected the error to name the manual override, got: %v", err)
	}
}

// An oidcConfig with an issuer but no client id cannot be completed by
// discovery, and the audience is what ArgoCD checks, so this must not default.
func TestDiscoverArgoCDOIDC_IssuerWithoutClientID(t *testing.T) {
	argocd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"oidcConfig":{"issuer":"https://idp.example.com"}}`))
	}))
	defer argocd.Close()

	if _, err := DiscoverArgoCDOIDC(context.Background(), testClient(), argocd.URL); err == nil {
		t.Fatal("expected an error for an oidcConfig without a client id")
	}
}
