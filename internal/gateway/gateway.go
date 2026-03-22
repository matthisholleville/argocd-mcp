package gateway

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/matthisholleville/argocd-mcp/internal/auth"
)

// Gateway proxies API requests to ArgoCD.
type Gateway struct {
	baseURL string
	token   string // static fallback token (token mode)
	logger  *slog.Logger
	client  *http.Client
}

// NewGateway creates a Gateway targeting the given ArgoCD instance.
// When tlsInsecure is true, TLS certificate verification is skipped.
func NewGateway(baseURL, staticToken string, tlsInsecure bool, logger *slog.Logger) *Gateway {
	return &Gateway{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   staticToken,
		logger:  logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: tlsInsecure, //nolint:gosec // Configurable via ARGOCD_TLS_INSECURE
				},
			},
		},
	}
}

// ExecuteParams holds the input for an execute_operation call.
type ExecuteParams struct {
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	QueryParams map[string]string `json:"query_params,omitempty"`
	Body        string            `json:"body,omitempty"`
}

// ExecuteResult is returned to the LLM after proxying the request.
type ExecuteResult struct {
	Status     int             `json:"status"`
	StatusText string          `json:"status_text"`
	Body       json.RawMessage `json:"body"`
}

// Execute proxies an API request to ArgoCD.
// In OAuth mode, forwards the user's Dex id_token directly (aud:argo-cd-cli).
// In token mode, uses the static ARGOCD_TOKEN.
func (g *Gateway) Execute(ctx context.Context, params ExecuteParams) (json.RawMessage, error) {
	targetURL, err := g.buildURL(params.Path, params.QueryParams)
	if err != nil {
		return nil, fmt.Errorf("build URL: %w", err)
	}

	var bodyReader io.Reader
	if params.Body != "" {
		bodyReader = strings.NewReader(params.Body)
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(params.Method), targetURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Use the user's OAuth token if available, otherwise static token.
	token := g.token
	if userToken, ok := auth.GetBearerToken(ctx); ok {
		token = userToken
		if claims := auth.ParseTokenClaims(userToken); claims != nil {
			g.logger.Info("argocd api call",
				slog.String("user", claims.Email),
				slog.String("method", params.Method),
				slog.String("path", params.Path),
			)
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	req.Header.Set("Accept", "application/json")
	if params.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	result := ExecuteResult{
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		Body:       json.RawMessage(respBody),
	}

	out, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return out, nil
}

func (g *Gateway) buildURL(path string, queryParams map[string]string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	u, err := url.Parse(g.baseURL + path)
	if err != nil {
		return "", err
	}

	if len(queryParams) > 0 {
		q := u.Query()
		for k, v := range queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	return u.String(), nil
}
