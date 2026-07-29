package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateClientRedirect(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		allowed []string
		wantErr bool
	}{
		{name: "loopback ipv4 ephemeral port", raw: "http://127.0.0.1:53421/callback"},
		{name: "loopback ipv6", raw: "http://[::1]:9382/callback"},
		{name: "localhost", raw: "http://localhost:8085/auth/callback"},
		{name: "https loopback", raw: "https://127.0.0.1:9382/cb"},
		{name: "explicitly allowed", raw: "https://claude.ai/api/mcp/auth_callback",
			allowed: []string{"https://claude.ai/api/mcp/auth_callback"}},

		{name: "arbitrary external host", raw: "https://evil.example.com/steal", wantErr: true},
		{name: "not the allowlisted value", raw: "https://claude.ai/other",
			allowed: []string{"https://claude.ai/api/mcp/auth_callback"}, wantErr: true},
		{name: "non-http scheme", raw: "javascript:alert(1)", wantErr: true},
		{name: "custom scheme", raw: "cursor://callback", wantErr: true},
		// A host merely *containing* a loopback name is not loopback.
		{name: "lookalike host", raw: "http://localhost.evil.example.com/cb", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateClientRedirect(tc.raw, tc.allowed)
			if tc.wantErr && err == nil {
				t.Fatalf("expected %q to be rejected", tc.raw)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected %q to be accepted, got %v", tc.raw, err)
			}
		})
	}
}

// The regression test for the open redirect: /authorize must refuse to sign a
// redirect target it would not be willing to deliver a code to. Without this,
// an attacker gets /oauth/callback to hand them a victim's authorization code,
// which the unauthenticated /token then exchanges into an ArgoCD bearer.
func TestHandleAuthorize_RejectsUnauthorizedRedirect(t *testing.T) {
	codec, err := NewStateCodec()
	if err != nil {
		t.Fatalf("NewStateCodec: %v", err)
	}
	h := HandleAuthorize(AuthorizeOptions{
		AuthorizationEndpoint: "https://idp.example.com/authorize",
		ClientID:              "cid",
		Scopes:                "openid",
		CallbackURL:           "https://mcp.example.com/oauth/callback",
		StateCodec:            codec,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&state=x&redirect_uri=https%3A%2F%2Fevil.example.com%2Fsteal", nil)
	h(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an off-allowlist redirect_uri, got %d (Location %q)",
			rec.Code, rec.Header().Get("Location"))
	}
}

func TestHandleAuthorize_AllowsLoopbackRedirect(t *testing.T) {
	codec, err := NewStateCodec()
	if err != nil {
		t.Fatalf("NewStateCodec: %v", err)
	}
	h := HandleAuthorize(AuthorizeOptions{
		AuthorizationEndpoint: "https://idp.example.com/authorize",
		ClientID:              "cid",
		Scopes:                "openid",
		CallbackURL:           "https://mcp.example.com/oauth/callback",
		StateCodec:            codec,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/authorize?response_type=code&state=x&redirect_uri=http%3A%2F%2F127.0.0.1%3A53421%2Fcallback", nil)
	h(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 for a loopback redirect_uri, got %d", rec.Code)
	}
}
