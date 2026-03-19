package config

import (
	"fmt"
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
	// DisableWrite excludes disruptive endpoints (POST, PUT, PATCH, DELETE)
	// from search results and blocks their execution.
	DisableWrite bool
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Transport:     getEnvOrDefault("MCP_TRANSPORT", "stdio"),
		Addr:          getEnvOrDefault("MCP_ADDR", ":8080"),
		ArgoCDBaseURL: os.Getenv("ARGOCD_BASE_URL"),
		ArgoCDToken:   os.Getenv("ARGOCD_TOKEN"),
		SpecURL:       os.Getenv("ARGOCD_SPEC_URL"),
		AuthMode:          getEnvOrDefault("AUTH_MODE", "token"),
		ServerBaseURL:     os.Getenv("SERVER_BASE_URL"),
		DexClientID:       getEnvOrDefault("DEX_CLIENT_ID", "argo-cd-cli"),
		EmbeddingsEnabled: parseBool("EMBEDDINGS_ENABLED", false),
		OllamaURL:         getEnvOrDefault("OLLAMA_URL", "http://localhost:11434/api"),
		EmbeddingsModel:   getEnvOrDefault("EMBEDDINGS_MODEL", "nomic-embed-text"),
		DisableWrite:      parseBool("DISABLE_WRITE", false),
	}

	var errs []string

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

	if len(errs) > 0 {
		return nil, fmt.Errorf("configuration errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	if cfg.SpecURL == "" {
		cfg.SpecURL = strings.TrimRight(cfg.ArgoCDBaseURL, "/") + "/swagger.json"
	}
	if cfg.ServerBaseURL == "" {
		cfg.ServerBaseURL = "http://localhost" + cfg.Addr
	}

	return cfg, nil
}

func parseBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
