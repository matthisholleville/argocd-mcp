package auth

import (
	"encoding/json"
	"net/http"
	"time"
)

type registerRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
}

// HandleRegister serves POST /register.
// Returns the configured Dex client_id and echoes back the client's
// redirect_uris so the MCP client knows its registration was accepted.
func HandleRegister(clientID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		resp := map[string]any{
			"client_id":                  clientID,
			"client_id_issued_at":        time.Now().Unix(),
			"token_endpoint_auth_method": "none",
		}

		if len(req.RedirectURIs) > 0 {
			resp["redirect_uris"] = req.RedirectURIs
		}
		if req.ClientName != "" {
			resp["client_name"] = req.ClientName
		}
		if len(req.GrantTypes) > 0 {
			resp["grant_types"] = req.GrantTypes
		}
		if len(req.ResponseTypes) > 0 {
			resp["response_types"] = req.ResponseTypes
		}
		if req.Scope != "" {
			resp["scope"] = req.Scope
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}
