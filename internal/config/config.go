package config

import (
	"fmt"
	"math"
	"os"
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
	// AuthMode is "token" (static token, default) or "oauth" (Dex SSO).
	AuthMode string
	// ServerBaseURL is the public URL of this server (for OAuth metadata).
	// Only used when AuthMode == "oauth". Defaults to http://localhost:{port}.
	ServerBaseURL string
	// DexClientID is the Dex static client ID. Must be registered in ArgoCD's
	// dex.config with the correct redirect_uris. Defaults to "argo-cd-cli".
	DexClientID string
	// Embeddings enables semantic vector search via Ollama.
	// When false, falls back to keyword search.
	EmbeddingsEnabled bool
	// OllamaURL is the Ollama API base URL.
	OllamaURL string
	// EmbeddingsModel is the Ollama embedding model name.
	EmbeddingsModel string
	// TLSInsecure disables TLS certificate verification when connecting to ArgoCD.
	// Defaults to false (secure). Set to true only when ArgoCD uses self-signed
	// certificates that cannot be added to the trust store. Prefer CABundlePath
	// in production — it keeps verification on while trusting a custom PKI.
	TLSInsecure bool
	// CABundlePath points at a PEM-encoded CA bundle on disk. When set, the
	// file is appended to the system trust pool for every outbound HTTP client
	// the server uses (OpenAPI fetcher, API gateway, Dex OAuth token proxy).
	// This is the recommended way to run against ArgoCD served behind a
	// private PKI (Vault, internal CA, etc.) without disabling TLS verification.
	CABundlePath string
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

	cfg := &Config{
		Transport:         getEnvOrDefault("MCP_TRANSPORT", "stdio"),
		Addr:              getEnvOrDefault("MCP_ADDR", ":8080"),
		ArgoCDBaseURL:     os.Getenv("ARGOCD_BASE_URL"),
		ArgoCDToken:       os.Getenv("ARGOCD_TOKEN"),
		SpecURL:           os.Getenv("ARGOCD_SPEC_URL"),
		AuthMode:          getEnvOrDefault("AUTH_MODE", "token"),
		ServerBaseURL:     os.Getenv("SERVER_BASE_URL"),
		DexClientID:       getEnvOrDefault("DEX_CLIENT_ID", "argo-cd-cli"),
		EmbeddingsEnabled: embeddingsEnabled,
		OllamaURL:         getEnvOrDefault("OLLAMA_URL", "http://localhost:11434/api"),
		EmbeddingsModel:   getEnvOrDefault("EMBEDDINGS_MODEL", "nomic-embed-text"),
		TLSInsecure:       tlsInsecure,
		CABundlePath:      os.Getenv("ARGOCD_CA_BUNDLE"),
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
	if cfg.AuthMode != "token" && cfg.AuthMode != "oauth" {
		errs = append(errs, fmt.Sprintf("AUTH_MODE must be 'token' or 'oauth', got %q", cfg.AuthMode))
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
