package config

import (
	"maps"
	"strings"
	"testing"
)

func TestLoad_OIDCRequiresHTTP(t *testing.T) {
	envs := minimalEnv()
	envs["AUTH_MODE"] = "oidc"
	envs["MCP_TRANSPORT"] = "stdio"
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for oidc without http")
	}
	if !strings.Contains(err.Error(), "AUTH_MODE=oidc requires MCP_TRANSPORT=http") {
		t.Errorf("expected oidc/http error, got: %v", err)
	}
}

// No issuer and no client id is the discover-from-ArgoCD path, which is valid.
func TestLoad_OIDCNoExplicitProvider(t *testing.T) {
	t.Setenv("ARGOCD_BASE_URL", "https://argocd.example.com")
	t.Setenv("AUTH_MODE", "oidc")
	t.Setenv("MCP_TRANSPORT", "http")
	t.Setenv("ARGOCD_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AuthMode != "oidc" {
		t.Errorf("expected oidc mode, got %q", cfg.AuthMode)
	}
	if cfg.OIDCScopes != "openid profile email groups" {
		t.Errorf("unexpected default scopes: %q", cfg.OIDCScopes)
	}
}

// Half a provider cannot be completed by discovery, since discovery reads the
// issuer and the client id together or not at all.
func TestLoad_OIDCPartialProvider(t *testing.T) {
	cases := map[string]map[string]string{
		"issuer without client id": {"OIDC_ISSUER": "https://idp.example.com"},
		"client id without issuer": {"OIDC_CLIENT_ID": "okta-client"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			envs := minimalEnv()
			envs["AUTH_MODE"] = "oidc"
			envs["MCP_TRANSPORT"] = "http"
			maps.Copy(envs, extra)
			setEnvs(t, envs)

			_, err := Load()
			if err == nil {
				t.Fatal("expected error for a half-configured provider")
			}
			if !strings.Contains(err.Error(), "OIDC_ISSUER and OIDC_CLIENT_ID") {
				t.Errorf("expected the pairing error, got: %v", err)
			}
		})
	}
}

func TestLoad_OIDCExplicitProvider(t *testing.T) {
	envs := minimalEnv()
	envs["AUTH_MODE"] = "oidc"
	envs["MCP_TRANSPORT"] = "http"
	envs["OIDC_ISSUER"] = "https://idp.example.com"
	envs["OIDC_CLIENT_ID"] = "okta-client"
	envs["OIDC_CLIENT_SECRET"] = "s3cret"
	envs["OIDC_SCOPES"] = "openid email"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDCIssuer != "https://idp.example.com" || cfg.OIDCClientID != "okta-client" {
		t.Errorf("unexpected provider: %q / %q", cfg.OIDCIssuer, cfg.OIDCClientID)
	}
	if cfg.OIDCClientSecret != "s3cret" {
		t.Errorf("unexpected client secret: %q", cfg.OIDCClientSecret)
	}
	if cfg.OIDCScopes != "openid email" {
		t.Errorf("unexpected scopes: %q", cfg.OIDCScopes)
	}
}

// Malformed claims JSON would be rejected by the provider mid-flow, so it is
// caught at startup instead.
func TestLoad_OIDCInvalidRequestedClaims(t *testing.T) {
	envs := minimalEnv()
	envs["AUTH_MODE"] = "oidc"
	envs["MCP_TRANSPORT"] = "http"
	envs["OIDC_REQUESTED_ID_TOKEN_CLAIMS"] = `{"id_token":`
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid claims JSON")
	}
	if !strings.Contains(err.Error(), "OIDC_REQUESTED_ID_TOKEN_CLAIMS") {
		t.Errorf("expected the claims error, got: %v", err)
	}
}

func TestLoad_OIDCValidRequestedClaims(t *testing.T) {
	envs := minimalEnv()
	envs["AUTH_MODE"] = "oidc"
	envs["MCP_TRANSPORT"] = "http"
	envs["OIDC_REQUESTED_ID_TOKEN_CLAIMS"] = `{"id_token":{"groups":{"essential":true}}}`
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDCRequestedIDTokenClaims != `{"id_token":{"groups":{"essential":true}}}` {
		t.Errorf("unexpected claims: %q", cfg.OIDCRequestedIDTokenClaims)
	}
}
