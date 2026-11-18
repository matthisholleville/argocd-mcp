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
)

func TestResolveOAuthProxy_ForwardsTLSInsecureToTokenExchange(t *testing.T) {
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
		{"oauth", config.Config{AuthMode: "oauth", ArgoCDBaseURL: "https://argocd.example.com", DexClientID: "argo-cd-cli", TLSInsecure: true}},
		{"oidc", config.Config{AuthMode: "oidc", ArgoCDBaseURL: "https://argocd.example.com", OIDCIssuer: provider.URL, OIDCClientID: "argo-cd", ServerBaseURL: "https://mcp.example.com", TLSInsecure: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy, err := resolveOAuthProxy(context.Background(), &tt.cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatalf("resolveOAuthProxy: %v", err)
			}
			if !proxy.token.TLSInsecure {
				t.Fatal("ARGOCD_TLS_INSECURE not forwarded to the token exchange")
			}
		})
	}
}
