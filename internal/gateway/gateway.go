package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/matthisholleville/argocd-mcp/internal/auth"
	"github.com/matthisholleville/argocd-mcp/internal/httputil"
)

// Gateway proxies API requests to ArgoCD.
type Gateway struct {
	baseURL string
	token   string // static fallback token (token mode)
	logger  *slog.Logger
	client  *http.Client
}

// NewGateway creates a Gateway targeting the given ArgoCD instance.
// When tlsInsecure is true, TLS certificate verification is skipped. When
// caBundlePath is non-empty, the PEM file is appended to the system trust
// pool — preferred over tlsInsecure for private PKI deployments.
func NewGateway(baseURL, staticToken string, tlsInsecure bool, caBundlePath string, logger *slog.Logger) (*Gateway, error) {
	client, err := httputil.NewClient(httputil.ClientOptions{
		Timeout:      30 * time.Second,
		TLSInsecure:  tlsInsecure,
		CABundlePath: caBundlePath,
	})
	if err != nil {
		return nil, fmt.Errorf("build gateway http client: %w", err)
	}
	return &Gateway{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   staticToken,
		logger:  logger,
		client:  client,
	}, nil
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

	const maxResponseSize = 10 << 20 // 10 MB
	respBody, err := httputil.ReadBody(resp.Body, maxResponseSize)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Determine how to represent the response body in the JSON result.
	// Content-Type is deliberately ignored in favour of byte-level validation
	// because ArgoCD may return NDJSON with Content-Type: application/json.
	var body json.RawMessage
	switch {
	case len(respBody) == 0:
		body = json.RawMessage("null")
	case json.Valid(respBody):
		body = respBody
	default:
		wrapped, err := json.Marshal(string(respBody))
		if err != nil {
			return nil, fmt.Errorf("marshal non-json body: %w", err)
		}
		body = wrapped
	}

	result := ExecuteResult{
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		Body:       body,
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
