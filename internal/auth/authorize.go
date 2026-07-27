package auth

import (
	"net/http"
	"net/url"
)

// DexScopes are the scopes ArgoCD's bundled Dex expects.
const DexScopes = "openid profile email groups offline_access"

// AuthorizeOptions configures the /authorize proxy.
type AuthorizeOptions struct {
	// AuthorizationEndpoint is the provider's authorization endpoint.
	AuthorizationEndpoint string
	// ClientID replaces whatever client_id the MCP client sent.
	ClientID string
	// Scopes replaces the client's scope. Providers disagree on what they
	// accept, so this is configurable rather than hardcoded.
	Scopes string
	// RequestedIDTokenClaims is forwarded as the OIDC `claims` request
	// parameter when non-empty. Some providers only put a claim in the id_token
	// when it is explicitly requested (ArgoCD's own oidc.config does the same
	// via requestedIDTokenClaims), and ArgoCD RBAC needs the groups claim.
	RequestedIDTokenClaims string
	// CallbackURL, when set, replaces the client's redirect_uri with this
	// server's own callback, the client's redirect and state travelling through
	// StateCodec instead. MCP clients bind an ephemeral loopback port that
	// cannot be pre-registered, which Dex tolerates for public clients (RFC
	// 8252 loopback) but most providers reject; with this, a single redirect URI
	// is registered once, at the provider.
	CallbackURL string
	// StateCodec is required when CallbackURL is set.
	StateCodec *StateCodec
}

// HandleAuthorize serves GET /authorize.
// Redirects to the provider's authorization endpoint, forwarding all OAuth params.
func HandleAuthorize(opts AuthorizeOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		q.Set("client_id", opts.ClientID)
		q.Set("scope", opts.Scopes)
		if opts.RequestedIDTokenClaims != "" {
			q.Set("claims", opts.RequestedIDTokenClaims)
		}

		if opts.CallbackURL != "" {
			clientRedirect := q.Get("redirect_uri")
			if clientRedirect == "" {
				http.Error(w, "redirect_uri is required", http.StatusBadRequest)
				return
			}
			state, err := opts.StateCodec.Encode(clientRedirect, q.Get("state"))
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			q.Set("redirect_uri", opts.CallbackURL)
			q.Set("state", state)
		}

		target, err := url.Parse(opts.AuthorizationEndpoint)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		target.RawQuery = q.Encode()

		http.Redirect(w, r, target.String(), http.StatusFound)
	}
}
