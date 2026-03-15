package openapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// FetchAndParse fetches the ArgoCD Swagger spec and returns the parsed endpoints.
func FetchAndParse(ctx context.Context, specURL, token string, logger *slog.Logger) ([]Endpoint, error) {
	logger.Info("fetching ArgoCD OpenAPI spec", slog.String("url", specURL))

	raw, err := fetchSpec(ctx, specURL, token)
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

func fetchSpec(ctx context.Context, specURL, token string) (json.RawMessage, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // ArgoCD often uses self-signed certs
			},
		},
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return json.RawMessage(body), nil
}
