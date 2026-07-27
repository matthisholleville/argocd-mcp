package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// StateCodec carries a client's redirect_uri and state through the upstream
// provider's state parameter, so the provider only ever sees this server's
// callback. Contents are signed, not encrypted: the provider and the browser
// both see them, and the signature is what stops a caller from injecting an
// arbitrary redirect target.
type StateCodec struct {
	key []byte
}

type statePayload struct {
	ClientRedirect string `json:"cr"`
	ClientState    string `json:"cs"`
	Nonce          string `json:"n"`
}

// NewStateCodec generates a per-process signing key. Logins in flight across a
// restart fail closed and the client retries, which is preferable to persisting
// a key or sharing one between replicas.
func NewStateCodec() (*StateCodec, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate state signing key: %w", err)
	}
	return &StateCodec{key: key}, nil
}

// Encode packs the client's redirect_uri and state into a signed opaque string.
func (c *StateCodec) Encode(clientRedirect, clientState string) (string, error) {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate state nonce: %w", err)
	}

	payload, err := json.Marshal(statePayload{
		ClientRedirect: clientRedirect,
		ClientState:    clientState,
		Nonce:          base64.RawURLEncoding.EncodeToString(nonce),
	})
	if err != nil {
		return "", fmt.Errorf("marshal state: %w", err)
	}

	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + c.sign(body), nil
}

// Decode verifies the signature and returns the client's redirect_uri and state.
func (c *StateCodec) Decode(state string) (clientRedirect, clientState string, err error) {
	body, sig, found := strings.Cut(state, ".")
	if !found {
		return "", "", fmt.Errorf("state is not in the expected format")
	}
	if !hmac.Equal([]byte(sig), []byte(c.sign(body))) {
		return "", "", fmt.Errorf("state signature does not verify")
	}

	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", "", fmt.Errorf("decode state: %w", err)
	}
	var payload statePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("unmarshal state: %w", err)
	}
	if payload.ClientRedirect == "" {
		return "", "", fmt.Errorf("state carries no client redirect_uri")
	}

	return payload.ClientRedirect, payload.ClientState, nil
}

func (c *StateCodec) sign(body string) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
