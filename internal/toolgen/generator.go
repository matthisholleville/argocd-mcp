package toolgen

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

// Executor abstracts the gateway's Execute method to avoid circular imports.
type Executor interface {
	Execute(ctx context.Context, params ExecuteParams) (json.RawMessage, error)
}

// ExecuteParams mirrors gateway.ExecuteParams.
type ExecuteParams struct {
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	QueryParams map[string]string `json:"query_params,omitempty"`
	Body        string            `json:"body,omitempty"`
}

// GeneratedTool holds a generated MCP tool and its handler.
type GeneratedTool struct {
	Tool    mcp.Tool
	Handler server.ToolHandlerFunc
}

// GenerateAll creates one MCP tool per endpoint with typed parameters.
// Endpoints without an operationId are skipped.
// If disableWrite is true, write methods (POST/PUT/PATCH/DELETE) are skipped as defence-in-depth.
func GenerateAll(endpoints []openapi.Endpoint, exec Executor, limiter ratelimit.Limiter, auditor *audit.Logger, disableWrite bool) []GeneratedTool {
	var tools []GeneratedTool
	names := make([]string, 0, len(endpoints))

	// First pass: collect names for dedup.
	for _, ep := range endpoints {
		if ep.OperationID == "" {
			names = append(names, "")
			continue
		}
		names = append(names, ToToolName(ep.OperationID))
	}
	names = DeduplicateNames(names)

	writeMethodSet := map[string]bool{
		"POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	}

	// Second pass: build tools.
	for i, ep := range endpoints {
		name := names[i]
		if name == "" {
			continue
		}
		// Defence-in-depth: skip write methods even if caller already filtered.
		if disableWrite && writeMethodSet[strings.ToUpper(ep.Method)] {
			continue
		}

		opts := []mcp.ToolOption{mcp.WithDescription(ep.Summary)}
		opts = append(opts, BuildToolOptions(ep)...)
		opts = append(opts, annotationsForMethod(ep.Method)...)

		tool := mcp.NewTool(name, opts...)
		handler := buildHandler(ep, exec, limiter, auditor, name)

		tools = append(tools, GeneratedTool{Tool: tool, Handler: handler})
	}

	return tools
}

func buildHandler(ep openapi.Endpoint, exec Executor, limiter ratelimit.Limiter, auditor *audit.Logger, toolName string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		user := userFromContext(ctx)

		// Rate limit check.
		if !limiter.Allow(user) {
			if auditor != nil {
				auditor.LogExecute(ctx, audit.Entry{
					Tool:    toolName,
					User:    user,
					Method:  ep.Method,
					Path:    ep.Path,
					Blocked: true,
				})
			}
			return mcp.NewToolResultError("rate limit exceeded: too many requests, please slow down"), nil
		}

		// Build path with parameter substitution.
		path := ep.Path
		queryParams := make(map[string]string)

		for _, p := range ep.Parameters {
			val := req.GetString(p.Name, "")
			if val == "" {
				continue
			}
			switch p.In {
			case "path":
				path = strings.ReplaceAll(path, "{"+p.Name+"}", val)
			case "query":
				queryParams[p.Name] = val
			}
		}

		// Validate all path placeholders were substituted.
		if strings.Contains(path, "{") {
			return mcp.NewToolResultError(fmt.Sprintf("missing required path parameter in %q", ep.Path)), nil
		}

		// Assemble body from body properties.
		body, bodyErr := assembleBody(req, ep.BodyProperties)
		if bodyErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid body: %v", bodyErr)), nil
		}

		start := time.Now()
		result, err := exec.Execute(ctx, ExecuteParams{
			Method:      ep.Method,
			Path:        path,
			QueryParams: queryParams,
			Body:        body,
		})
		duration := time.Since(start)

		if auditor != nil {
			entry := audit.Entry{
				Tool:     toolName,
				User:     user,
				Method:   ep.Method,
				Path:     path,
				Duration: duration,
			}
			if err != nil {
				entry.Error = err.Error()
			} else {
				entry.StatusCode = extractStatusCode(result)
			}
			auditor.LogExecute(ctx, entry)
		}

		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("ArgoCD API error: %v", err)), nil
		}

		return mcp.NewToolResultText(string(result)), nil
	}
}

// assembleBody builds a JSON body string from the request's body property values.
func assembleBody(req mcp.CallToolRequest, bodyProps []openapi.BodyProperty) (string, error) {
	if len(bodyProps) == 0 {
		return "", nil
	}

	fields := make(map[string]any)
	for _, bp := range bodyProps {
		val := req.GetString(bp.Name, "")
		if val == "" {
			continue
		}

		switch bp.Type {
		case "boolean":
			if val == "true" {
				fields[bp.Name] = true
			} else if val == "false" {
				fields[bp.Name] = false
			} else {
				fields[bp.Name] = val
			}
		case "integer":
			// Try to parse as number, fall back to string.
			var n json.Number
			if err := json.Unmarshal([]byte(val), &n); err == nil {
				fields[bp.Name] = n
			} else {
				fields[bp.Name] = val
			}
		case "number":
			var n json.Number
			if err := json.Unmarshal([]byte(val), &n); err == nil {
				fields[bp.Name] = n
			} else {
				fields[bp.Name] = val
			}
		case "object", "array":
			// Try to parse as JSON, fall back to string.
			var raw json.RawMessage
			if err := json.Unmarshal([]byte(val), &raw); err == nil {
				fields[bp.Name] = raw
			} else {
				fields[bp.Name] = val
			}
		default:
			fields[bp.Name] = val
		}
	}

	if len(fields) == 0 {
		return "", nil
	}

	b, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("marshal body: %w", err)
	}
	return string(b), nil
}

// annotationsForMethod returns MCP tool annotations based on the HTTP method.
// GET/HEAD/OPTIONS are read-only; DELETE is destructive; PUT/PATCH are idempotent.
func annotationsForMethod(method string) []mcp.ToolOption {
	switch strings.ToUpper(method) {
	case "GET", "HEAD", "OPTIONS":
		return []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		}
	case "DELETE":
		return []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		}
	case "PUT":
		return []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		}
	default: // POST, PATCH
		return []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		}
	}
}

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
func extractStatusCode(raw json.RawMessage) *int {
	var envelope struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Status == 0 {
		return nil
	}
	return &envelope.Status
}
