package auth

import (
	"net/http"
	"net/url"
)

// HandleAuthorize serves GET /authorize.
// Redirects to ArgoCD's Dex authorization endpoint, forwarding all OAuth params.
func HandleAuthorize(dexAuthURL, clientID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		q.Set("client_id", clientID)

		// Dex requires "openid" scope. Always override whatever the client sent.
		q.Set("scope", "openid profile email groups offline_access")

		target, err := url.Parse(dexAuthURL)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		target.RawQuery = q.Encode()

		http.Redirect(w, r, target.String(), http.StatusFound)
	}
}
