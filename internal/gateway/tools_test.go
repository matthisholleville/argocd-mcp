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
	return NewGateway(srv.URL, "test-token", slog.Default())
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

// --- execute_operation tests ---

func TestHandleExecute_DisableWriteBlocksWriteMethods(t *testing.T) {
	writeMethods := []string{"POST", "PUT", "PATCH", "DELETE"}
	gw := newTestGateway(t)
	handler := handleExecute(gw, true, nil)

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
	handler := handleExecute(gw, true, nil)

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
	handler := handleExecute(gw, false, nil)

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

func TestHandleExecute_AuditLogsSuccessfulCall(t *testing.T) {
	var buf bytes.Buffer
	auditor := newTestAuditor(&buf)
	gw := newTestGateway(t)
	handler := handleExecute(gw, false, auditor)

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
	handler := handleExecute(gw, true, auditor)

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
	// Blocked requests must not have status_code.
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
	// ResultCount must be 0 on error (HIGH fix #2).
	if entry["result_count"] != float64(0) {
		t.Errorf("expected result_count=0 on error, got %v", entry["result_count"])
	}
}
