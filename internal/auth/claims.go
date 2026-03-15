package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// TokenClaims holds basic identity claims from a JWT.
type TokenClaims struct {
	Sub    string   `json:"sub"`
	Email  string   `json:"email"`
	Name   string   `json:"name"`
	Groups []string `json:"groups"`
}

// ParseTokenClaims decodes the JWT payload without verification.
// Used only for audit logging — not for security decisions.
func ParseTokenClaims(token string) *TokenClaims {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	return &claims
}
