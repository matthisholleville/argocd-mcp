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
	// AllowedClientRedirects are non-loopback redirect_uri values accepted in
	// addition to loopback. See ValidateClientRedirect: substituting our own
	// callback removes the provider's redirect check, so we have to do it.
	AllowedClientRedirects []string
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
			if err := ValidateClientRedirect(clientRedirect, opts.AllowedClientRedirects); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
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
		// Merge rather than assign: providers bake parameters into the
		// authorization_endpoint they advertise (Azure AD B2C's ?p=<policy>), and
		// dropping them makes every login fail at the IdP.
		target.RawQuery = mergeQuery(target.Query(), q).Encode()

		http.Redirect(w, r, target.String(), http.StatusFound)
	}
}

// mergeQuery overlays over onto base, leaving base's other parameters intact.
func mergeQuery(base, over url.Values) url.Values {
	for k, vals := range over {
		base[k] = vals
	}
	return base
}
