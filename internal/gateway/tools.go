package gateway

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matthisholleville/argocd-mcp/internal/openapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Searcher abstracts the search backend (keyword or vector).
type Searcher interface {
	Search(ctx context.Context, query string, maxResults int) ([]openapi.Endpoint, error)
}

// RegisterMCPTools registers the two meta-tools on the MCP server.
func RegisterMCPTools(srv *server.MCPServer, endpointCount int, searcher Searcher, gw *Gateway) {
	srv.AddTool(searchTool(endpointCount), handleSearch(searcher))
	srv.AddTool(executeTool(), handleExecute(gw))
}

func searchTool(endpointCount int) mcp.Tool {
	return mcp.NewTool(
		"search_operations",
		mcp.WithDescription(fmt.Sprintf(
			"Search the ArgoCD API (%d endpoints). "+
				"Returns matching endpoints with method, path, summary, and parameters. "+
				"Use this to discover which API calls are available before executing them.",
			endpointCount,
		)),
		mcp.WithString("query",
			mcp.Description("Search query (e.g. 'sync application', 'get logs', 'list projects')"),
		),
	)
}

func executeTool() mcp.Tool {
	return mcp.NewTool(
		"execute_operation",
		mcp.WithDescription(
			"Execute an ArgoCD API operation. "+
				"Use search_operations first to discover the correct method, path, and parameters.",
		),
		mcp.WithString("method",
			mcp.Required(),
			mcp.Description("HTTP method: GET, POST, PUT, PATCH, DELETE"),
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

func handleSearch(searcher Searcher) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := req.GetString("query", "")

		results, err := searcher.Search(ctx, query, 20)
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

func handleExecute(gw *Gateway) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		method, err := req.RequireString("method")
		if err != nil {
			return mcp.NewToolResultError("method is required"), nil
		}
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError("path is required"), nil
		}
		body := req.GetString("body", "")

		queryParams := make(map[string]string)
		if raw := req.GetString("query_params", ""); raw != "" {
			if err := json.Unmarshal([]byte(raw), &queryParams); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid query_params JSON: %v", err)), nil
			}
		}

		result, err := gw.Execute(ctx, ExecuteParams{
			Method:      method,
			Path:        path,
			QueryParams: queryParams,
			Body:        body,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("execute: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	}
}
