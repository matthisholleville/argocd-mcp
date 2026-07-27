package auth

import (
	"encoding/json"
	"net/http"
	"time"
	"net/url"
	"strings"

	"github.com/matthisholleville/argocd-mcp/internal/httputil"
)

// TokenOptions configures the /token proxy.
type TokenOptions struct {
	// TokenEndpoint is the provider's token endpoint.
	TokenEndpoint string
	// ClientID replaces whatever client_id the MCP client sent.
	ClientID string
	// ClientSecret authenticates the exchange when the provider treats this as
	// a confidential client. Empty for public clients such as Dex's
	// argo-cd-cli, which rely on PKCE alone. The secret stays server-side: MCP
	// clients still register as public against this server.
	ClientSecret string
}

// HandleToken serves POST /token.
// Proxies the token exchange to the provider's token endpoint.
// Swaps the id_token into the access_token field because ArgoCD validates
// the id_token (not the access_token) as the Bearer token.
func HandleToken(opts TokenOptions) http.HandlerFunc {
	httpClient := &http.Client{Timeout: 15 * time.Second}

	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error":             "invalid_request",
				"error_description": "could not parse request body",
			})
			return
		}

		form := url.Values{}
		for k, vals := range r.Form {
			form[k] = vals
		}
		form.Set("client_id", opts.ClientID)
		if opts.ClientSecret != "" {
			form.Set("client_secret", opts.ClientSecret)
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, opts.TokenEndpoint,
			strings.NewReader(form.Encode()))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error":             "upstream_error",
				"error_description": "could not reach the provider token endpoint",
			})
			return
		}
		defer resp.Body.Close()

		const maxTokenResponseSize = 1 << 20 // 1 MB
		body, err := httputil.ReadBody(resp.Body, maxTokenResponseSize)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "read_error"})
			return
		}

		// If the provider returned an error, forward it as-is.
		if resp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(body)
			return
		}

		// Swap id_token into access_token so the MCP client uses it as Bearer.
		// ArgoCD validates the id_token JWT, not the opaque access_token.
		var tokenResp map[string]any
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
			return
		}

		if idToken, ok := tokenResp["id_token"].(string); ok && idToken != "" {
			tokenResp["access_token"] = idToken
			tokenResp["token_type"] = "Bearer"
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(tokenResp)
	}
}
