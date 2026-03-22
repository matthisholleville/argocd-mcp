package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/matthisholleville/argocd-mcp/internal/audit"
	"github.com/matthisholleville/argocd-mcp/internal/auth"
	"github.com/matthisholleville/argocd-mcp/internal/openapi"
	"github.com/matthisholleville/argocd-mcp/internal/ratelimit"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Searcher abstracts the search backend (keyword or vector).
type Searcher interface {
	Search(ctx context.Context, query string, maxResults int) ([]openapi.Endpoint, error)
}

// RegisterMCPTools registers the two meta-tools on the MCP server.
// When disableWrite is true, write operations (POST, PUT, PATCH, DELETE) are blocked.
// When allowed is non-nil, only endpoints in the set can be executed.
// When auditor is non-nil, every tool call is logged.
// ToolOptions holds configuration that affects tool descriptions shown to the LLM.
type ToolOptions struct {
	EndpointCount    int
	DisableWrite     bool
	AllowedResources []string
}

func RegisterMCPTools(srv *server.MCPServer, opts ToolOptions, searcher Searcher, gw *Gateway, allowed *openapi.AllowedEndpoints, limiter ratelimit.Limiter, auditor *audit.Logger) {
	srv.AddTool(searchTool(opts), handleSearch(searcher, auditor))
	srv.AddTool(executeTool(opts), handleExecute(gw, opts.DisableWrite, allowed, limiter, auditor))
}

func searchTool(opts ToolOptions) mcp.Tool {
	desc := fmt.Sprintf(
		"Search the ArgoCD API (%d endpoints). "+
			"Returns matching endpoints with method, path, summary, and parameters. "+
			"Use this to discover which API calls are available before executing them.",
		opts.EndpointCount,
	)
	if len(opts.AllowedResources) > 0 {
		desc += fmt.Sprintf(" Restricted to: %s.", strings.Join(opts.AllowedResources, ", "))
	}
	if opts.DisableWrite {
		desc += " Read-only mode: write operations are disabled."
	}

	return mcp.NewTool(
		"search_operations",
		mcp.WithDescription(desc),
		mcp.WithString("query",
			mcp.Description("Search query (e.g. 'sync application', 'get logs', 'list projects')"),
		),
	)
}

func executeTool(opts ToolOptions) mcp.Tool {
	methodDesc := "HTTP method: GET, POST, PUT, PATCH, DELETE"
	if opts.DisableWrite {
		methodDesc = "HTTP method: GET, HEAD, OPTIONS (write operations are disabled)"
	}

	desc := "Execute an ArgoCD API operation. " +
		"Use search_operations first to discover the correct method, path, and parameters."
	if len(opts.AllowedResources) > 0 {
		desc += fmt.Sprintf(" Only these resources are allowed: %s.", strings.Join(opts.AllowedResources, ", "))
	}

	return mcp.NewTool(
		"execute_operation",
		mcp.WithDescription(desc),
		mcp.WithString("method",
			mcp.Required(),
			mcp.Description(methodDesc),
		),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("API path (e.g. /api/v1/applications). Replace path parameters with actual values."),
		),
		mcp.WithString("query_params",
			mcp.Description("Query parameters as a JSON object string (optional). Example: '{\"refresh\":\"true\"}'"),
		),
		mcp.WithString("body",
			mcp.Description("JSON request body as a string (optional, for POST/PUT/PATCH)"),
		),
	)
}

func handleSearch(searcher Searcher, auditor *audit.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")
		start := time.Now()

		results, err := searcher.Search(ctx, query, 20)
		duration := time.Since(start)

		if auditor != nil {
			entry := audit.Entry{
				Tool:     "search_operations",
				Query:    query,
				Duration: duration,
				User:     userFromContext(ctx),
			}
			if err != nil {
				entry.Error = fmt.Sprintf("search failed: %v", err)
			} else {
				entry.ResultCount = len(results)
			}
			auditor.LogSearch(ctx, entry)
		}

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
		}

		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal results: %v", err)), nil
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func isWriteMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

func handleExecute(gw *Gateway, disableWrite bool, allowed *openapi.AllowedEndpoints, limiter ratelimit.Limiter, auditor *audit.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		method, err := req.RequireString("method")
		if err != nil {
			return mcp.NewToolResultError("method is required"), nil
		}

		path := req.GetString("path", "")
		if path == "" {
			return mcp.NewToolResultError("path is required"), nil
		}

		user := userFromContext(ctx)

		if disableWrite && isWriteMethod(method) {
			if auditor != nil {
				auditor.LogExecute(ctx, audit.Entry{
					Tool:    "execute_operation",
					User:    user,
					Method:  strings.ToUpper(method),
					Path:    path,
					Blocked: true,
				})
			}
			return mcp.NewToolResultError(fmt.Sprintf(
				"write operations are disabled: %s method is not allowed (DISABLE_WRITE=true)",
				strings.ToUpper(method),
			)), nil
		}

		if !allowed.IsAllowed(method, path) {
			if auditor != nil {
				auditor.LogExecute(ctx, audit.Entry{
					Tool:    "execute_operation",
					User:    user,
					Method:  strings.ToUpper(method),
					Path:    path,
					Blocked: true,
				})
			}
			return mcp.NewToolResultError(fmt.Sprintf(
				"operation not allowed: %s %s is outside the permitted resource scope (ALLOWED_RESOURCES)",
				strings.ToUpper(method), path,
			)), nil
		}
		if !limiter.Allow(user) {
			if auditor != nil {
				auditor.LogExecute(ctx, audit.Entry{
					Tool:    "execute_operation",
					User:    user,
					Method:  strings.ToUpper(method),
					Path:    path,
					Blocked: true,
				})
			}
			return mcp.NewToolResultError("rate limit exceeded: too many requests, please slow down"), nil
		}

		body := req.GetString("body", "")

		queryParams := make(map[string]string)
		if raw := req.GetString("query_params", ""); raw != "" {
			if err := json.Unmarshal([]byte(raw), &queryParams); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid query_params JSON: %v", err)), nil
			}
		}

		start := time.Now()
		result, err := gw.Execute(ctx, ExecuteParams{
			Method:      method,
			Path:        path,
			QueryParams: queryParams,
			Body:        body,
		})
		duration := time.Since(start)

		if auditor != nil {
			entry := audit.Entry{
				Tool:     "execute_operation",
				User:     user,
				Method:   strings.ToUpper(method),
				Path:     path,
				Duration: duration,
			}
			if err != nil {
				entry.Error = fmt.Sprintf("execute: %v", err)
			} else {
				entry.StatusCode = extractStatusCode(result)
			}
			auditor.LogExecute(ctx, entry)
		}

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("execute: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	}
}

// userFromContext extracts the user identity from the context.
// In OAuth mode, returns the email from the JWT claims.
// In token mode (no bearer token in context), returns "static-token".
func userFromContext(ctx context.Context) string {
	token, ok := auth.GetBearerToken(ctx)
	if !ok {
		return "static-token"
	}
	claims := auth.ParseTokenClaims(token)
	if claims == nil || claims.Email == "" {
		return "static-token"
	}
	return claims.Email
}

// extractStatusCode reads the status field from the gateway JSON response.
// Returns nil when the status cannot be determined.
func extractStatusCode(raw json.RawMessage) *int {
	var resp struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.Status == 0 {
		return nil
	}
	return &resp.Status
}
