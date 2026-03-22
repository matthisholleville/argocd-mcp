package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matthisholleville/argocd-mcp/internal/audit"
	"github.com/matthisholleville/argocd-mcp/internal/openapi"
	"github.com/matthisholleville/argocd-mcp/internal/ratelimit"
	"github.com/mark3labs/mcp-go/mcp"
)

// newTestGateway creates a Gateway backed by a fake HTTP server that always returns 200 OK.
func newTestGateway(t *testing.T) *Gateway {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return NewGateway(srv.URL, "test-token", false, slog.Default())
}

func buildCallToolRequest(t *testing.T, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = make(map[string]any)
	if err := json.Unmarshal(raw, &req.Params.Arguments); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return req
}

func newTestAuditor(buf *bytes.Buffer) *audit.Logger {
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	return audit.New(slog.New(handler))
}

func parseAuditLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("parse audit line: %v (raw: %s)", err, line)
		}
		entries = append(entries, entry)
	}
	return entries
}

// stubSearcher implements Searcher for testing.
type stubSearcher struct {
	results []openapi.Endpoint
	err     error
}

func (s *stubSearcher) Search(_ context.Context, _ string, _ int) ([]openapi.Endpoint, error) {
	return s.results, s.err
}

var noopLimiter = ratelimit.New(context.Background(), 0, 0)

// --- execute_operation: DISABLE_WRITE tests ---

func TestHandleExecute_DisableWriteBlocksWriteMethods(t *testing.T) {
	writeMethods := []string{"POST", "PUT", "PATCH", "DELETE"}
	gw := newTestGateway(t)
	handler := handleExecute(gw, true, nil, noopLimiter, nil)

	for _, method := range writeMethods {
		t.Run(method, func(t *testing.T) {
			req := buildCallToolRequest(t, map[string]any{
				"method": method,
				"path":   "/api/v1/applications",
			})

			result, err := handler(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error result for blocked write method")
			}
			text := result.Content[0].(mcp.TextContent).Text
			if !strings.Contains(text, "disabled") {
				t.Errorf("expected 'disabled' in error message, got: %s", text)
			}
		})
	}
}

func TestHandleExecute_DisableWriteAllowsReadMethods(t *testing.T) {
	readMethods := []string{"GET", "HEAD", "OPTIONS"}
	gw := newTestGateway(t)
	handler := handleExecute(gw, true, nil, noopLimiter, nil)

	for _, method := range readMethods {
		t.Run(method, func(t *testing.T) {
			req := buildCallToolRequest(t, map[string]any{
				"method": method,
				"path":   "/api/v1/applications",
			})

			result, err := handler(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsError {
				text := result.Content[0].(mcp.TextContent).Text
				if strings.Contains(text, "disabled") {
					t.Errorf("read method %s should not be blocked, got: %s", method, text)
				}
			}
		})
	}
}

func TestHandleExecute_WriteAllowedWhenNotDisabled(t *testing.T) {
	gw := newTestGateway(t)
	handler := handleExecute(gw, false, nil, noopLimiter, nil)

	req := buildCallToolRequest(t, map[string]any{
		"method": "DELETE",
		"path":   "/api/v1/applications/foo",
	})

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := result.Content[0].(mcp.TextContent).Text
		if strings.Contains(text, "disabled") {
			t.Errorf("write should not be blocked when disableWrite=false, got: %s", text)
		}
	}
}

// --- execute_operation: ALLOWED_RESOURCES tests ---

func TestHandleExecute_AllowedResourcesBlocksOutOfScope(t *testing.T) {
	gw := newTestGateway(t)
	allowed := openapi.NewAllowedEndpoints([]openapi.Endpoint{
		{Method: "GET", Path: "/api/v1/version"},
	})
	handler := handleExecute(gw, false, allowed, noopLimiter, nil)

	req := buildCallToolRequest(t, map[string]any{
		"method": "GET",
		"path":   "/api/v1/applications",
	})

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for out-of-scope endpoint")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "ALLOWED_RESOURCES") {
		t.Errorf("expected ALLOWED_RESOURCES in error, got: %s", text)
	}
}

func TestHandleExecute_AllowedResourcesPermitsInScope(t *testing.T) {
	gw := newTestGateway(t)
	allowed := openapi.NewAllowedEndpoints([]openapi.Endpoint{
		{Method: "GET", Path: "/api/v1/applications"},
		{Method: "GET", Path: "/api/v1/applications/{name}"},
	})
	handler := handleExecute(gw, false, allowed, noopLimiter, nil)

	req := buildCallToolRequest(t, map[string]any{
		"method": "GET",
		"path":   "/api/v1/applications/myapp",
	})

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := result.Content[0].(mcp.TextContent).Text
		t.Errorf("expected success for in-scope endpoint, got: %s", text)
	}
}

func TestHandleExecute_DisableWritePlusAllowedResources(t *testing.T) {
	gw := newTestGateway(t)
	// Allowed: only GET on version (simulating DISABLE_WRITE + ALLOWED_RESOURCES=VersionService)
	allowed := openapi.NewAllowedEndpoints([]openapi.Endpoint{
		{Method: "GET", Path: "/api/v1/version"},
	})
	handler := handleExecute(gw, true, allowed, noopLimiter, nil)

	// POST should be blocked by DISABLE_WRITE first.
	t.Run("write_blocked_by_disable_write", func(t *testing.T) {
		req := buildCallToolRequest(t, map[string]any{
			"method": "POST",
			"path":   "/api/v1/applications",
		})
		result, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error")
		}
		text := result.Content[0].(mcp.TextContent).Text
		if !strings.Contains(text, "DISABLE_WRITE") {
			t.Errorf("expected DISABLE_WRITE error, got: %s", text)
		}
	})

	// GET on applications should be blocked by ALLOWED_RESOURCES.
	t.Run("read_blocked_by_allowed_resources", func(t *testing.T) {
		req := buildCallToolRequest(t, map[string]any{
			"method": "GET",
			"path":   "/api/v1/applications",
		})
		result, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error")
		}
		text := result.Content[0].(mcp.TextContent).Text
		if !strings.Contains(text, "ALLOWED_RESOURCES") {
			t.Errorf("expected ALLOWED_RESOURCES error, got: %s", text)
		}
	})

	// GET on version should succeed.
	t.Run("version_allowed", func(t *testing.T) {
		req := buildCallToolRequest(t, map[string]any{
			"method": "GET",
			"path":   "/api/v1/version",
		})
		result, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			text := result.Content[0].(mcp.TextContent).Text
			t.Errorf("expected success, got: %s", text)
		}
	})
}

func TestHandleExecute_AllowedResourcesAuditLogsBlocked(t *testing.T) {
	var buf bytes.Buffer
	auditor := newTestAuditor(&buf)
	gw := newTestGateway(t)
	allowed := openapi.NewAllowedEndpoints([]openapi.Endpoint{
		{Method: "GET", Path: "/api/v1/version"},
	})
	handler := handleExecute(gw, false, allowed, noopLimiter, auditor)

	req := buildCallToolRequest(t, map[string]any{
		"method": "GET",
		"path":   "/api/v1/applications",
	})

	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := parseAuditLines(t, &buf)
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	if entries[0]["blocked"] != true {
		t.Errorf("expected blocked=true, got %v", entries[0]["blocked"])
	}
}

// --- execute_operation: audit tests ---

func TestHandleExecute_AuditLogsSuccessfulCall(t *testing.T) {
	var buf bytes.Buffer
	auditor := newTestAuditor(&buf)
	gw := newTestGateway(t)
	handler := handleExecute(gw, false, nil, noopLimiter, auditor)

	req := buildCallToolRequest(t, map[string]any{
		"method": "GET",
		"path":   "/api/v1/applications",
	})

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}

	entries := parseAuditLines(t, &buf)
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry["tool"] != "execute_operation" {
		t.Errorf("expected tool=execute_operation, got %v", entry["tool"])
	}
	if entry["method"] != "GET" {
		t.Errorf("expected method=GET, got %v", entry["method"])
	}
	if entry["path"] != "/api/v1/applications" {
		t.Errorf("expected path, got %v", entry["path"])
	}
	if entry["blocked"] != false {
		t.Errorf("expected blocked=false, got %v", entry["blocked"])
	}
	if entry["user"] != "static-token" {
		t.Errorf("expected user=static-token (no OAuth context), got %v", entry["user"])
	}
}

func TestHandleExecute_AuditLogsBlockedWrite(t *testing.T) {
	var buf bytes.Buffer
	auditor := newTestAuditor(&buf)
	gw := newTestGateway(t)
	handler := handleExecute(gw, true, nil, noopLimiter, auditor)

	req := buildCallToolRequest(t, map[string]any{
		"method": "DELETE",
		"path":   "/api/v1/applications/myapp",
	})

	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries := parseAuditLines(t, &buf)
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry["blocked"] != true {
		t.Errorf("expected blocked=true, got %v", entry["blocked"])
	}
	if entry["method"] != "DELETE" {
		t.Errorf("expected method=DELETE, got %v", entry["method"])
	}
	if _, ok := entry["status_code"]; ok {
		t.Error("blocked requests should not have status_code")
	}
}

// --- search_operations tests ---

func TestHandleSearch_AuditLogsSuccessfulSearch(t *testing.T) {
	var buf bytes.Buffer
	auditor := newTestAuditor(&buf)
	searcher := &stubSearcher{
		results: []openapi.Endpoint{
			{Method: "POST", Path: "/api/v1/applications/{name}/sync", Summary: "Sync an application"},
			{Method: "GET", Path: "/api/v1/applications", Summary: "List applications"},
		},
	}
	handler := handleSearch(searcher, auditor)

	req := buildCallToolRequest(t, map[string]any{"query": "sync application"})
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}

	entries := parseAuditLines(t, &buf)
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry["tool"] != "search_operations" {
		t.Errorf("expected tool=search_operations, got %v", entry["tool"])
	}
	if entry["query"] != "sync application" {
		t.Errorf("expected query='sync application', got %v", entry["query"])
	}
	if entry["result_count"] != float64(2) {
		t.Errorf("expected result_count=2, got %v", entry["result_count"])
	}
	if entry["user"] != "static-token" {
		t.Errorf("expected user=static-token, got %v", entry["user"])
	}
}

func TestHandleSearch_AuditLogsErrorWithZeroResultCount(t *testing.T) {
	var buf bytes.Buffer
	auditor := newTestAuditor(&buf)
	searcher := &stubSearcher{
		err: fmt.Errorf("index not ready"),
	}
	handler := handleSearch(searcher, auditor)

	req := buildCallToolRequest(t, map[string]any{"query": "sync"})
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}

	entries := parseAuditLines(t, &buf)
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry["level"] != "ERROR" {
		t.Errorf("expected level=ERROR, got %v", entry["level"])
	}
	if entry["result_count"] != float64(0) {
		t.Errorf("expected result_count=0 on error, got %v", entry["result_count"])
	}
}

// --- execute_operation: rate limiting tests ---

func TestHandleExecute_RateLimitBlocks(t *testing.T) {
	gw := newTestGateway(t)
	// burst=1: only 1 request passes, then blocked.
	lim := ratelimit.New(context.Background(), 1, 1)
	handler := handleExecute(gw, false, nil, lim, nil)

	req := buildCallToolRequest(t, map[string]any{
		"method": "GET",
		"path":   "/api/v1/applications",
	})

	// First request passes.
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("first request should succeed")
	}

	// Second request is rate limited.
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
