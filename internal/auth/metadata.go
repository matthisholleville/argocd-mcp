package auth

import (
	"encoding/json"
	"net/http"
)

type authServerMetadata struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	RegistrationEndpoint          string   `json:"registration_endpoint"`
	ResponseTypesSupported        []string `json:"response_types_supported"`
	GrantTypesSupported           []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
}

type protectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
}

// HandleAuthServerMetadata serves GET /.well-known/oauth-authorization-server.
func HandleAuthServerMetadata(serverBaseURL string) http.HandlerFunc {
	meta := authServerMetadata{
		Issuer:                        serverBaseURL,
		AuthorizationEndpoint:         serverBaseURL + "/authorize",
		TokenEndpoint:                 serverBaseURL + "/token",
		RegistrationEndpoint:          serverBaseURL + "/register",
		ResponseTypesSupported:        []string{"code"},
		GrantTypesSupported:           []string{"authorization_code"},
		CodeChallengeMethodsSupported: []string{"S256"},
	}
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, meta)
	}
}

// HandleProtectedResourceMetadata serves GET /.well-known/oauth-protected-resource.
func HandleProtectedResourceMetadata(serverBaseURL string) http.HandlerFunc {
	meta := protectedResourceMetadata{
		Resource:               serverBaseURL,
		AuthorizationServers:   []string{serverBaseURL},
		BearerMethodsSupported: []string{"header"},
	}
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, meta)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
