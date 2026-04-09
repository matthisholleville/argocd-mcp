package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/matthisholleville/argocd-mcp/internal/httputil"
)

// FetchAndParse fetches the ArgoCD Swagger spec and returns the parsed endpoints.
func FetchAndParse(ctx context.Context, specURL, token string, tlsInsecure bool, caBundlePath string, logger *slog.Logger) ([]Endpoint, error) {
	logger.Info("fetching ArgoCD OpenAPI spec", slog.String("url", specURL))

	raw, err := fetchSpec(ctx, specURL, token, tlsInsecure, caBundlePath)
	if err != nil {
		return nil, fmt.Errorf("fetch spec: %w", err)
	}

	endpoints, err := ParseSpec(raw)
	if err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}

	logger.Info("ArgoCD OpenAPI spec parsed", slog.Int("endpoints", len(endpoints)))
	return endpoints, nil
}

func fetchSpec(ctx context.Context, specURL, token string, tlsInsecure bool, caBundlePath string) (json.RawMessage, error) {
	// Timeout is governed by the ctx deadline passed by the caller.
	client, err := httputil.NewClient(httputil.ClientOptions{
		TLSInsecure:  tlsInsecure,
		CABundlePath: caBundlePath,
	})
	if err != nil {
		return nil, fmt.Errorf("build http client: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, specURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	const maxSpecSize = 50 << 20 // 50 MB
	body, err := httputil.ReadBody(resp.Body, maxSpecSize)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	return json.RawMessage(body), nil
}
