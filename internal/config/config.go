package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	// Transport is "stdio" (default) or "http".
	Transport string
	// Addr is the HTTP listen address when Transport == "http".
	Addr string
	// ArgoCD base URL, e.g. "https://argocd.example.com".
	ArgoCDBaseURL string
	// ArgoCD API token (used when AuthMode == "token").
	ArgoCDToken string
	// SpecURL overrides the default spec URL ({base}/swagger.json).
	SpecURL string
	// AuthMode is "token" (static token, default), "oauth" (ArgoCD's bundled
	// Dex) or "oidc" (the external provider ArgoCD trusts through oidc.config).
	AuthMode string
	// ServerBaseURL is the public URL of this server (for OAuth metadata).
	// Unused when AuthMode == "token". Defaults to http://localhost:{port}.
	ServerBaseURL string
	// DexClientID is the Dex static client ID. Must be registered in ArgoCD's
	// dex.config with the correct redirect_uris. Defaults to "argo-cd-cli".
	DexClientID string
	// OIDCIssuer is the issuer URL of the provider, used for endpoint
	// discovery. Empty means: read it from ArgoCD's own oidc.config.
	OIDCIssuer string
	// OIDCClientID is the client the MCP authenticates users with. Its id must
	// be an audience ArgoCD accepts (its oidc.config clientID or cliClientID),
	// otherwise ArgoCD rejects the resulting token. Empty means: read it from
	// ArgoCD's own oidc.config.
	OIDCClientID string
	// OIDCClientSecret authenticates the token exchange when the provider
	// treats the client as confidential. Empty for public/PKCE clients.
	OIDCClientSecret string
	// OIDCScopes are the scopes requested in oidc mode.
	OIDCScopes string
	// OIDCScopesExplicit records that OIDC_SCOPES was set, so discovery knows it
	// must not override the operator with ArgoCD's own requestedScopes.
	OIDCScopesExplicit bool
	// OIDCProxyCallback makes this server, rather than the MCP client, the
	// redirect target registered at the provider. On by default: MCP clients
	// bind loopback ports that cannot be pre-registered, and unlike Dex most
	// providers require an exact redirect_uri match.
	OIDCProxyCallback bool
	// OIDCRequestedIDTokenClaims is raw JSON forwarded as the OIDC `claims`
	// request parameter, mirroring ArgoCD's requestedIDTokenClaims. Needed when
	// the provider only emits a claim on request, e.g. groups for ArgoCD RBAC.
	OIDCRequestedIDTokenClaims string
	// OIDCAllowedRedirectURIs are non-loopback client redirect_uri values this
	// server will hand an authorization code to. Loopback is always accepted;
	// anything else has to be listed here, because in proxy-callback mode the
	// provider no longer validates the client's redirect_uri.
	OIDCAllowedRedirectURIs []string
	// OIDCStateKey is the shared secret the callback state is signed with. Empty
	// generates a per-process key, which only works with a single replica: any
	// replica can serve the browser's callback and only the one that minted the
	// state can verify it.
	OIDCStateKey string
	// Embeddings enables semantic vector search via Ollama.
	// When false, falls back to keyword search.
	EmbeddingsEnabled bool
	// OllamaURL is the Ollama API base URL.
	OllamaURL string
	// EmbeddingsModel is the Ollama embedding model name.
	EmbeddingsModel string
	// TLSInsecure disables TLS certificate verification when connecting to ArgoCD.
	// Defaults to false (secure). Set to true only when ArgoCD uses self-signed
	// certificates that cannot be added to the trust store.
	TLSInsecure bool
	// DisableWrite excludes disruptive endpoints (POST, PUT, PATCH, DELETE)
	// from search results and blocks their execution.
	DisableWrite bool
	// AllowedResources restricts which ArgoCD resource types are exposed.
	// Comma-separated list of OpenAPI tags (e.g. "ApplicationService,VersionService").
	// When set, only matching endpoints are searchable and executable.
	// Empty means all resources are allowed.
	AllowedResources []string
	// ToolMode is "search" (default, 2 meta-tools) or "generated" (1 tool per endpoint).
	ToolMode string
	// RateLimit is the maximum number of execute_operation requests per second
	// per user. Set to 0 to disable rate limiting.
	RateLimit float64
	// RateLimitBurst is the maximum burst size. Defaults to RateLimit if 0.
	RateLimitBurst int
	// AuditLog enables structured audit logging for every tool call.
	// Logs are emitted as JSON to stderr alongside other server logs.
	AuditLog bool
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	var errs []string

	embeddingsEnabled, err := parseBool("EMBEDDINGS_ENABLED", false)
	errs = appendIfErr(errs, err)
	tlsInsecure, err := parseBool("ARGOCD_TLS_INSECURE", false)
	errs = appendIfErr(errs, err)
	disableWrite, err := parseBool("DISABLE_WRITE", false)
	errs = appendIfErr(errs, err)
	auditLog, err := parseBool("AUDIT_LOG", true)
	errs = appendIfErr(errs, err)
	rateLimit, err := parseFloat("RATE_LIMIT", 0)
	errs = appendIfErr(errs, err)
	rateLimitBurst, err := parseInt("RATE_LIMIT_BURST", 0)
	errs = appendIfErr(errs, err)
	oidcProxyCallback, err := parseBool("OIDC_PROXY_CALLBACK", true)
	errs = appendIfErr(errs, err)

	cfg := &Config{
		Transport:     getEnvOrDefault("MCP_TRANSPORT", "stdio"),
		Addr:          getEnvOrDefault("MCP_ADDR", ":8080"),
		ArgoCDBaseURL: os.Getenv("ARGOCD_BASE_URL"),
		ArgoCDToken:   os.Getenv("ARGOCD_TOKEN"),
		SpecURL:       os.Getenv("ARGOCD_SPEC_URL"),
		AuthMode:      getEnvOrDefault("AUTH_MODE", "token"),
		ServerBaseURL: os.Getenv("SERVER_BASE_URL"),
		DexClientID:   getEnvOrDefault("DEX_CLIENT_ID", "argo-cd-cli"),

		OIDCIssuer:                 os.Getenv("OIDC_ISSUER"),
		OIDCClientID:               os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret:           os.Getenv("OIDC_CLIENT_SECRET"),
		OIDCScopes:                 getEnvOrDefault("OIDC_SCOPES", "openid profile email groups"),
		OIDCScopesExplicit:         os.Getenv("OIDC_SCOPES") != "",
		OIDCProxyCallback:          oidcProxyCallback,
		OIDCRequestedIDTokenClaims: os.Getenv("OIDC_REQUESTED_ID_TOKEN_CLAIMS"),
		OIDCAllowedRedirectURIs:    parseCSV("OIDC_ALLOWED_REDIRECT_URIS"),
		OIDCStateKey:               os.Getenv("OIDC_STATE_KEY"),

		EmbeddingsEnabled: embeddingsEnabled,
		OllamaURL:         getEnvOrDefault("OLLAMA_URL", "http://localhost:11434/api"),
		EmbeddingsModel:   getEnvOrDefault("EMBEDDINGS_MODEL", "nomic-embed-text"),
		TLSInsecure:       tlsInsecure,
		DisableWrite:      disableWrite,
		AllowedResources:  parseCSV("ALLOWED_RESOURCES"),
		ToolMode:          getEnvOrDefault("TOOL_MODE", "search"),
		RateLimit:         rateLimit,
		RateLimitBurst:    rateLimitBurst,
		AuditLog:          auditLog,
	}

	if cfg.ArgoCDBaseURL == "" {
		errs = append(errs, "ARGOCD_BASE_URL is required")
	}
	if cfg.AuthMode != "token" && cfg.AuthMode != "oauth" && cfg.AuthMode != "oidc" {
		errs = append(errs, fmt.Sprintf("AUTH_MODE must be 'token', 'oauth' or 'oidc', got %q", cfg.AuthMode))
	}
	if cfg.AuthMode == "token" && cfg.ArgoCDToken == "" {
		errs = append(errs, "ARGOCD_TOKEN is required when AUTH_MODE=token")
	}
	if cfg.Transport != "stdio" && cfg.Transport != "http" {
		errs = append(errs, fmt.Sprintf("MCP_TRANSPORT must be 'stdio' or 'http', got %q", cfg.Transport))
	}
	if cfg.AuthMode == "oauth" && cfg.Transport != "http" {
		errs = append(errs, "AUTH_MODE=oauth requires MCP_TRANSPORT=http")
	}
	if cfg.AuthMode == "oidc" && cfg.Transport != "http" {
		errs = append(errs, "AUTH_MODE=oidc requires MCP_TRANSPORT=http")
	}
	// An issuer without a client id (or the reverse) cannot be completed from
	// ArgoCD: discovery reads both or neither.
	if cfg.AuthMode == "oidc" && (cfg.OIDCIssuer == "") != (cfg.OIDCClientID == "") {
		errs = append(errs, "OIDC_ISSUER and OIDC_CLIENT_ID must be set together, or both left empty to read them from ArgoCD")
	}
	// In proxy-callback mode SERVER_BASE_URL is the redirect URI registered at
	// the provider, not just advertisement metadata, so defaulting it to
	// localhost yields redirect_uri_mismatch on every login with nothing in our
	// own logs to explain it.
	if cfg.AuthMode == "oidc" && cfg.OIDCProxyCallback && cfg.ServerBaseURL == "" {
		errs = append(errs, "SERVER_BASE_URL is required when AUTH_MODE=oidc and OIDC_PROXY_CALLBACK is enabled: it is the redirect URI registered at the provider")
	}
	// Without openid the provider mints no id_token, and the id_token is the
	// only thing ArgoCD accepts as a Bearer.
	if cfg.AuthMode == "oidc" && !slices.Contains(strings.Fields(cfg.OIDCScopes), "openid") {
		errs = append(errs, fmt.Sprintf("OIDC_SCOPES must include 'openid', got %q", cfg.OIDCScopes))
	}
	if cfg.OIDCRequestedIDTokenClaims != "" {
		// The OIDC `claims` request parameter is an object; a bare scalar or array
		// is valid JSON that every provider rejects with invalid_request.
		var claims map[string]any
		if err := json.Unmarshal([]byte(cfg.OIDCRequestedIDTokenClaims), &claims); err != nil {
			errs = append(errs, "OIDC_REQUESTED_ID_TOKEN_CLAIMS must be a JSON object, e.g. {\"id_token\":{\"groups\":{\"essential\":true}}}")
		}
	}
	if cfg.ToolMode != "search" && cfg.ToolMode != "generated" {
		errs = append(errs, fmt.Sprintf("TOOL_MODE must be 'search' or 'generated', got %q", cfg.ToolMode))
	}
	if cfg.RateLimit < 0 {
		errs = append(errs, "RATE_LIMIT must be >= 0")
	}
	if cfg.RateLimitBurst < 0 {
		errs = append(errs, "RATE_LIMIT_BURST must be >= 0")
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("configuration errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	if cfg.SpecURL == "" {
		cfg.SpecURL = strings.TrimRight(cfg.ArgoCDBaseURL, "/") + "/swagger.json"
	}
	if cfg.ServerBaseURL == "" {
		cfg.ServerBaseURL = "http://localhost" + cfg.Addr
	}
	if cfg.RateLimitBurst == 0 && cfg.RateLimit > 0 {
		cfg.RateLimitBurst = int(math.Ceil(cfg.RateLimit))
	}

	return cfg, nil
}

func appendIfErr(errs []string, err error) []string {
	if err != nil {
		return append(errs, err.Error())
	}
	return errs
}

func parseFloat(key string, defaultVal float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal, fmt.Errorf("%s=%q is not a valid number", key, v)
	}
	return f, nil
}

func parseInt(key string, defaultVal int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal, fmt.Errorf("%s=%q is not a valid integer", key, v)
	}
	return i, nil
}

func parseBool(key string, defaultVal bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal, fmt.Errorf("%s=%q is not a valid boolean (expected true/false/1/0)", key, v)
	}
	return b, nil
}

func parseCSV(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
