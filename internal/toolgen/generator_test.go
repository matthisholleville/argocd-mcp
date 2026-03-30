package toolgen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/matthisholleville/argocd-mcp/internal/audit"
	"github.com/matthisholleville/argocd-mcp/internal/openapi"
	"github.com/matthisholleville/argocd-mcp/internal/ratelimit"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- mock executor ---

type mockExecutor struct {
	lastParams ExecuteParams
	result     json.RawMessage
	err        error
}

func (m *mockExecutor) Execute(_ context.Context, params ExecuteParams) (json.RawMessage, error) {
	m.lastParams = params
	return m.result, m.err
}

func newMockExecutor(status int) *mockExecutor {
	result, _ := json.Marshal(map[string]any{
		"status":      status,
		"status_text": fmt.Sprintf("%d OK", status),
		"body":        nil,
	})
	return &mockExecutor{result: result}
}

func buildCallToolRequest(t *testing.T, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func newTestAuditor(buf *bytes.Buffer) *audit.Logger {
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)
	return audit.New(logger)
}

func parseAuditLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("parse audit line: %v", err)
		}
		entries = append(entries, m)
	}
	return entries
}

var noopLimiter = ratelimit.New(context.Background(), 0, 0)

// --- GenerateAll tests ---

func TestGenerateAll_ToolDefinition(t *testing.T) {
	endpoints := []openapi.Endpoint{
		{
			OperationID: "ApplicationService_Sync",
			Method:      "POST",
			Path:        "/api/v1/applications/{name}/sync",
			Summary:     "Sync syncs an application to its target state",
			Parameters: []openapi.Parameter{
				{Name: "name", In: "path", Required: true, Type: "string", Description: "App name"},
			},
			BodyProperties: []openapi.BodyProperty{
				{Name: "revision", Type: "string", Description: "Target revision"},
				{Name: "dryRun", Type: "boolean"},
			},
		},
	}

	exec := newMockExecutor(200)
	tools := GenerateAll(endpoints, exec, noopLimiter, nil, false)

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	tool := tools[0].Tool
	if tool.Name != "argocd_application_sync" {
		t.Errorf("name = %q, want argocd_application_sync", tool.Name)
	}
	if tool.Description != "Sync syncs an application to its target state" {
		t.Errorf("description = %q", tool.Description)
	}

	// Verify params exist.
	for _, name := range []string{"name", "revision", "dryRun"} {
		if _, ok := tool.InputSchema.Properties[name]; !ok {
			t.Errorf("expected %q param in tool", name)
		}
	}

	// name should be required.
	if !contains(tool.InputSchema.Required, "name") {
		t.Error("name should be required")
	}
}

func TestGenerateAll_SkipsNoOperationID(t *testing.T) {
	endpoints := []openapi.Endpoint{
		{OperationID: "ApplicationService_List", Method: "GET", Path: "/api/v1/applications", Summary: "List"},
		{OperationID: "", Method: "GET", Path: "/api/v1/version", Summary: "Version"},
	}

	tools := GenerateAll(endpoints, newMockExecutor(200), noopLimiter, nil, false)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool (skip empty operationId), got %d", len(tools))
	}
	if tools[0].Tool.Name != "argocd_application_list" {
		t.Errorf("name = %q", tools[0].Tool.Name)
	}
}

func TestGenerateAll_MultipleEndpoints(t *testing.T) {
	endpoints := []openapi.Endpoint{
		{OperationID: "ApplicationService_List", Method: "GET", Path: "/api/v1/applications", Summary: "List apps"},
		{OperationID: "ApplicationService_Sync", Method: "POST", Path: "/api/v1/applications/{name}/sync", Summary: "Sync app"},
		{OperationID: "ClusterService_List", Method: "GET", Path: "/api/v1/clusters", Summary: "List clusters"},
	}

	tools := GenerateAll(endpoints, newMockExecutor(200), noopLimiter, nil, false)
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, gt := range tools {
		names[gt.Tool.Name] = true
	}
	for _, want := range []string{"argocd_application_list", "argocd_application_sync", "argocd_cluster_list"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

// --- Handler tests ---

func TestHandler_PathSubstitution(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_Get",
		Method:      "GET",
		Path:        "/api/v1/applications/{name}",
		Summary:     "Get application",
		Parameters: []openapi.Parameter{
			{Name: "name", In: "path", Required: true, Type: "string"},
		},
	}

	exec := newMockExecutor(200)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, nil, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{"name": "frontend"})
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exec.lastParams.Path != "/api/v1/applications/frontend" {
		t.Errorf("path = %q, want /api/v1/applications/frontend", exec.lastParams.Path)
	}
	if exec.lastParams.Method != "GET" {
		t.Errorf("method = %q, want GET", exec.lastParams.Method)
	}
}

func TestHandler_MultiplePathParams(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_PodLogs",
		Method:      "GET",
		Path:        "/api/v1/applications/{name}/logs/{container}",
		Summary:     "Get pod logs",
		Parameters: []openapi.Parameter{
			{Name: "name", In: "path", Required: true, Type: "string"},
			{Name: "container", In: "path", Required: true, Type: "string"},
		},
	}

	exec := newMockExecutor(200)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, nil, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{"name": "myapp", "container": "main"})
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exec.lastParams.Path != "/api/v1/applications/myapp/logs/main" {
		t.Errorf("path = %q", exec.lastParams.Path)
	}
}

func TestHandler_QueryParams(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_List",
		Method:      "GET",
		Path:        "/api/v1/applications",
		Summary:     "List applications",
		Parameters: []openapi.Parameter{
			{Name: "refresh", In: "query", Type: "string"},
			{Name: "appNamespace", In: "query", Type: "string"},
		},
	}

	exec := newMockExecutor(200)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, nil, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{"refresh": "true", "appNamespace": "argocd"})
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exec.lastParams.QueryParams["refresh"] != "true" {
		t.Errorf("refresh = %q", exec.lastParams.QueryParams["refresh"])
	}
	if exec.lastParams.QueryParams["appNamespace"] != "argocd" {
		t.Errorf("appNamespace = %q", exec.lastParams.QueryParams["appNamespace"])
	}
}

func TestHandler_BodyAssembly(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_Sync",
		Method:      "POST",
		Path:        "/api/v1/applications/{name}/sync",
		Summary:     "Sync",
		Parameters: []openapi.Parameter{
			{Name: "name", In: "path", Required: true, Type: "string"},
		},
		BodyProperties: []openapi.BodyProperty{
			{Name: "revision", Type: "string"},
			{Name: "dryRun", Type: "boolean"},
			{Name: "prune", Type: "boolean"},
		},
	}

	exec := newMockExecutor(200)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, nil, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{
		"name":     "frontend",
		"revision": "HEAD",
		"dryRun":   "true",
		"prune":    "false",
	})
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify body was assembled as JSON.
	var body map[string]any
	if err := json.Unmarshal([]byte(exec.lastParams.Body), &body); err != nil {
		t.Fatalf("body is not JSON: %v (body=%q)", err, exec.lastParams.Body)
	}

	if body["revision"] != "HEAD" {
		t.Errorf("revision = %v", body["revision"])
	}
	if body["dryRun"] != true {
		t.Errorf("dryRun = %v (type %T)", body["dryRun"], body["dryRun"])
	}
	if body["prune"] != false {
		t.Errorf("prune = %v", body["prune"])
	}

	// Path should have name substituted, not in body.
	if exec.lastParams.Path != "/api/v1/applications/frontend/sync" {
		t.Errorf("path = %q", exec.lastParams.Path)
	}
}

func TestHandler_BodyAssembly_NestedJSON(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_Sync",
		Method:      "POST",
		Path:        "/api/v1/applications/{name}/sync",
		Summary:     "Sync",
		Parameters: []openapi.Parameter{
			{Name: "name", In: "path", Required: true, Type: "string"},
		},
		BodyProperties: []openapi.BodyProperty{
			{Name: "strategy", Type: "object"},
			{Name: "resources", Type: "array"},
		},
	}

	exec := newMockExecutor(200)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, nil, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{
		"name":      "frontend",
		"strategy":  `{"apply":{"force":true}}`,
		"resources": `[{"kind":"Deployment","name":"web"}]`,
	})
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(exec.lastParams.Body), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}

	// strategy should be parsed as nested JSON, not a string.
	strat, ok := body["strategy"].(map[string]any)
	if !ok {
		t.Fatalf("strategy should be object, got %T: %v", body["strategy"], body["strategy"])
	}
	if _, ok := strat["apply"]; !ok {
		t.Error("strategy.apply not found")
	}

	// resources should be parsed as array.
	res, ok := body["resources"].([]any)
	if !ok {
		t.Fatalf("resources should be array, got %T", body["resources"])
	}
	if len(res) != 1 {
		t.Errorf("expected 1 resource, got %d", len(res))
	}
}

func TestHandler_EmptyBody(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_Get",
		Method:      "GET",
		Path:        "/api/v1/applications/{name}",
		Summary:     "Get",
		Parameters: []openapi.Parameter{
			{Name: "name", In: "path", Required: true, Type: "string"},
		},
	}

	exec := newMockExecutor(200)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, nil, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{"name": "frontend"})
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exec.lastParams.Body != "" {
		t.Errorf("expected empty body for GET, got %q", exec.lastParams.Body)
	}
}

func TestHandler_RateLimitApplied(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_List",
		Method:      "GET",
		Path:        "/api/v1/applications",
		Summary:     "List",
	}

	exec := newMockExecutor(200)
	lim := ratelimit.New(context.Background(), 1, 1)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, lim, nil, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{})

	// First call passes.
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("first request should succeed")
	}

	// Second call is rate limited.
	result, err = handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("second request should be rate limited")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "rate limit") {
		t.Errorf("expected rate limit error, got: %s", text)
	}
}

func TestHandler_AuditLogToolName(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ClusterService_Get",
		Method:      "GET",
		Path:        "/api/v1/clusters/{name}",
		Summary:     "Get cluster",
		Parameters: []openapi.Parameter{
			{Name: "name", In: "path", Required: true, Type: "string"},
		},
	}

	var buf bytes.Buffer
	auditor := newTestAuditor(&buf)
	exec := newMockExecutor(200)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, auditor, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{"name": "prod"})
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := parseAuditLines(t, &buf)
	if len(entries) == 0 {
		t.Fatal("expected audit log entry")
	}

	entry := entries[0]
	if entry["tool"] != "argocd_cluster_get" {
		t.Errorf("audit tool = %v, want argocd_cluster_get", entry["tool"])
	}
	if entry["method"] != "GET" {
		t.Errorf("audit method = %v", entry["method"])
	}
	if entry["path"] != "/api/v1/clusters/prod" {
		t.Errorf("audit path = %v", entry["path"])
	}
}

func TestHandler_AuditLogRateLimitBlocked(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_Delete",
		Method:      "DELETE",
		Path:        "/api/v1/applications/{name}",
		Summary:     "Delete",
		Parameters: []openapi.Parameter{
			{Name: "name", In: "path", Required: true, Type: "string"},
		},
	}

	var buf bytes.Buffer
	auditor := newTestAuditor(&buf)
	exec := newMockExecutor(200)
	lim := ratelimit.New(context.Background(), 1, 1)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, lim, auditor, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{"name": "myapp"})

	// Exhaust rate limit.
	handler(context.Background(), req)
	handler(context.Background(), req)

	entries := parseAuditLines(t, &buf)
	// Find the blocked entry.
	var blocked bool
	for _, e := range entries {
		if b, ok := e["blocked"].(bool); ok && b {
			blocked = true
			if e["tool"] != "argocd_application_delete" {
				t.Errorf("blocked entry tool = %v", e["tool"])
			}
		}
	}
	if !blocked {
		t.Error("expected a blocked audit entry")
	}
}

func TestHandler_ExecuteError(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_List",
		Method:      "GET",
		Path:        "/api/v1/applications",
		Summary:     "List",
	}

	exec := &mockExecutor{err: fmt.Errorf("connection refused")}
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, nil, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{})
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "connection refused") {
		t.Errorf("expected error message, got: %s", text)
	}
}

func TestHandler_BodyAssembly_IntegerParam(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "Test_WithInt",
		Method:      "POST",
		Path:        "/test",
		Summary:     "Test",
		BodyProperties: []openapi.BodyProperty{
			{Name: "count", Type: "integer"},
		},
	}

	exec := newMockExecutor(200)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, nil, false)
	handler := tools[0].Handler

	req := buildCallToolRequest(t, map[string]any{"count": "42"})
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(exec.lastParams.Body), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	// json.Number should be preserved, not a string.
	if fmt.Sprintf("%v", body["count"]) != "42" {
		t.Errorf("count = %v", body["count"])
	}
}

func TestHandler_SkipEmptyOptionalParams(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_List",
		Method:      "GET",
		Path:        "/api/v1/applications",
		Summary:     "List",
		Parameters: []openapi.Parameter{
			{Name: "refresh", In: "query", Type: "string"},
		},
	}

	exec := newMockExecutor(200)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, nil, false)
	handler := tools[0].Handler

	// Call without providing the optional param.
	req := buildCallToolRequest(t, map[string]any{})
	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(exec.lastParams.QueryParams) != 0 {
		t.Errorf("expected no query params, got %v", exec.lastParams.QueryParams)
	}
}

func TestHandler_MissingPathParam(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_Get",
		Method:      "GET",
		Path:        "/api/v1/applications/{name}",
		Summary:     "Get",
		Parameters: []openapi.Parameter{
			{Name: "name", In: "path", Required: true, Type: "string"},
		},
	}

	exec := newMockExecutor(200)
	tools := GenerateAll([]openapi.Endpoint{ep}, exec, noopLimiter, nil, false)
	handler := tools[0].Handler

	// Call without providing the required path param.
	req := buildCallToolRequest(t, map[string]any{})
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing path param")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "missing required path parameter") {
		t.Errorf("unexpected error message: %s", text)
	}
}

// --- Annotation tests ---

func TestGenerateAll_DisableWriteExcludesWriteTools(t *testing.T) {
	endpoints := []openapi.Endpoint{
		{OperationID: "ApplicationService_List", Method: "GET", Path: "/api/v1/applications", Summary: "List"},
		{OperationID: "ApplicationService_Create", Method: "POST", Path: "/api/v1/applications", Summary: "Create"},
		{OperationID: "ApplicationService_Delete", Method: "DELETE", Path: "/api/v1/applications/{name}", Summary: "Delete",
			Parameters: []openapi.Parameter{{Name: "name", In: "path", Required: true, Type: "string"}}},
		{OperationID: "ClusterService_Get", Method: "GET", Path: "/api/v1/clusters/{name}", Summary: "Get cluster",
			Parameters: []openapi.Parameter{{Name: "name", In: "path", Required: true, Type: "string"}}},
	}

	tools := GenerateAll(endpoints, newMockExecutor(200), noopLimiter, nil, true) // disableWrite=true

	// Only GET tools should be generated.
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools with disableWrite, got %d", len(tools))
	}
	for _, gt := range tools {
		if gt.Tool.Name != "argocd_application_list" && gt.Tool.Name != "argocd_cluster_get" {
			t.Errorf("unexpected write tool registered: %s", gt.Tool.Name)
		}
	}
}

func TestAnnotations_GetIsReadOnly(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_List",
		Method:      "GET",
		Path:        "/api/v1/applications",
		Summary:     "List",
	}

	tools := GenerateAll([]openapi.Endpoint{ep}, newMockExecutor(200), noopLimiter, nil, false)
	ann := tools[0].Tool.Annotations

	if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint != true {
		t.Error("GET tool should have readOnlyHint=true")
	}
}

func TestAnnotations_DeleteIsDestructive(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_Delete",
		Method:      "DELETE",
		Path:        "/api/v1/applications/{name}",
		Summary:     "Delete",
		Parameters:  []openapi.Parameter{{Name: "name", In: "path", Required: true, Type: "string"}},
	}

	tools := GenerateAll([]openapi.Endpoint{ep}, newMockExecutor(200), noopLimiter, nil, false)
	ann := tools[0].Tool.Annotations

	if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint != false {
		t.Error("DELETE tool should have readOnlyHint=false")
	}
	if ann.DestructiveHint == nil || *ann.DestructiveHint != true {
		t.Error("DELETE tool should have destructiveHint=true")
	}
	if ann.IdempotentHint == nil || *ann.IdempotentHint != true {
		t.Error("DELETE tool should have idempotentHint=true")
	}
}

func TestAnnotations_PostIsWrite(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_Create",
		Method:      "POST",
		Path:        "/api/v1/applications",
		Summary:     "Create",
	}

	tools := GenerateAll([]openapi.Endpoint{ep}, newMockExecutor(200), noopLimiter, nil, false)
	ann := tools[0].Tool.Annotations

	if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint != false {
		t.Error("POST tool should have readOnlyHint=false")
	}
	if ann.DestructiveHint != nil && *ann.DestructiveHint != false {
		t.Error("POST tool should not be destructive")
	}
}

func TestAnnotations_PutIsIdempotent(t *testing.T) {
	ep := openapi.Endpoint{
		OperationID: "ApplicationService_Update",
		Method:      "PUT",
		Path:        "/api/v1/applications/{name}",
		Summary:     "Update",
		Parameters:  []openapi.Parameter{{Name: "name", In: "path", Required: true, Type: "string"}},
	}

	tools := GenerateAll([]openapi.Endpoint{ep}, newMockExecutor(200), noopLimiter, nil, false)
	ann := tools[0].Tool.Annotations

	if ann.IdempotentHint == nil || *ann.IdempotentHint != true {
		t.Error("PUT tool should have idempotentHint=true")
	}
	if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint != false {
		t.Error("PUT tool should have readOnlyHint=false")
	}
}
