package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matthisholleville/argocd-mcp/internal/config"
	"github.com/matthisholleville/argocd-mcp/internal/httputil"
)

func TestResolveOAuthProxy_ForwardsTLSConfigToTokenExchange(t *testing.T) {
	tlsConfig, err := httputil.TLSConfig(true, "")
	if err != nil {
		t.Fatal(err)
	}

	var provider *httptest.Server
	provider = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 provider.URL,
			"authorization_endpoint": provider.URL + "/authorize",
			"token_endpoint":         provider.URL + "/token",
		})
	}))
	t.Cleanup(provider.Close)

	tests := []struct {
		name string
		cfg  config.Config
	}{
		{"oauth", config.Config{AuthMode: "oauth", ArgoCDBaseURL: "https://argocd.example.com", DexClientID: "argo-cd-cli"}},
		{"oidc", config.Config{AuthMode: "oidc", ArgoCDBaseURL: "https://argocd.example.com", OIDCIssuer: provider.URL, OIDCClientID: "argo-cd", ServerBaseURL: "https://mcp.example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy, err := resolveOAuthProxy(context.Background(), &tt.cfg, tlsConfig, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatalf("resolveOAuthProxy: %v", err)
			}
			if proxy.token.TLSConfig != tlsConfig {
				t.Fatal("shared TLS config (ARGOCD_TLS_INSECURE, ARGOCD_CA_BUNDLE) not forwarded to the token exchange")
			}
		})
	}
}

func TestFrontend_NotReadyUntilAppIsSet(t *testing.T) {
	f := newFrontend()

	status := func(method, path string) int {
		rec := httptest.NewRecorder()
		f.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec.Code
	}

	tests := []struct {
		method, path string
		starting     int
		ready        int
	}{
		{http.MethodGet, "/healthz", http.StatusOK, http.StatusOK},
		{http.MethodGet, "/readyz", http.StatusServiceUnavailable, http.StatusOK},
		{http.MethodPost, "/mcp", http.StatusServiceUnavailable, http.StatusTeapot},
		{http.MethodGet, "/.well-known/oauth-authorization-server", http.StatusServiceUnavailable, http.StatusTeapot},
	}
	for _, tt := range tests {
		if got := status(tt.method, tt.path); got != tt.starting {
			t.Errorf("starting: %s %s = %d, want %d", tt.method, tt.path, got, tt.starting)
		}
	}

	f.setApp(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))

	for _, tt := range tests {
		if got := status(tt.method, tt.path); got != tt.ready {
			t.Errorf("ready: %s %s = %d, want %d", tt.method, tt.path, got, tt.ready)
		}
	}
}
