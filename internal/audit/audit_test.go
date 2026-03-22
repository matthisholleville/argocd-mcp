package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

func newTestLogger(buf *bytes.Buffer) *Logger {
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	return New(slog.New(handler))
}

func parseLogEntry(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("parse log entry: %v (raw: %s)", err, buf.String())
	}
	return entry
}

func TestLogExecute_BasicFields(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.LogExecute(context.Background(), Entry{
		Tool:       "execute_operation",
		Method:     "GET",
		Path:       "/api/v1/applications",
		StatusCode: IntPtr(200),
		Duration:   150 * time.Millisecond,
	})

	entry := parseLogEntry(t, &buf)

	if entry["msg"] != "audit" {
		t.Errorf("expected msg=audit, got %v", entry["msg"])
	}
	if entry["tool"] != "execute_operation" {
		t.Errorf("expected tool=execute_operation, got %v", entry["tool"])
	}
	if entry["method"] != "GET" {
		t.Errorf("expected method=GET, got %v", entry["method"])
	}
	if entry["path"] != "/api/v1/applications" {
		t.Errorf("expected path=/api/v1/applications, got %v", entry["path"])
	}
	if entry["status_code"] != float64(200) {
		t.Errorf("expected status_code=200, got %v", entry["status_code"])
	}
	if entry["blocked"] != false {
		t.Errorf("expected blocked=false, got %v", entry["blocked"])
	}
	if _, ok := entry["duration_ms"]; !ok {
		t.Error("expected duration_ms field")
	}
}

func TestLogExecute_WithUser(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.LogExecute(context.Background(), Entry{
		Tool:       "execute_operation",
		User:       "alice@example.com",
		Method:     "POST",
		Path:       "/api/v1/applications",
		StatusCode: IntPtr(200),
		Duration:   50 * time.Millisecond,
	})

	entry := parseLogEntry(t, &buf)

	if entry["user"] != "alice@example.com" {
		t.Errorf("expected user=alice@example.com, got %v", entry["user"])
	}
}

func TestLogExecute_Blocked(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.LogExecute(context.Background(), Entry{
		Tool:    "execute_operation",
		Method:  "DELETE",
		Path:    "/api/v1/applications/myapp",
		Blocked: true,
	})

	entry := parseLogEntry(t, &buf)

	if entry["blocked"] != true {
		t.Errorf("expected blocked=true, got %v", entry["blocked"])
	}
	if entry["method"] != "DELETE" {
		t.Errorf("expected method=DELETE, got %v", entry["method"])
	}
	// Blocked requests should not have a status code.
	if _, ok := entry["status_code"]; ok {
		t.Error("blocked requests should not have status_code")
	}
}

func TestLogExecute_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.LogExecute(context.Background(), Entry{
		Tool:     "execute_operation",
		Method:   "GET",
		Path:     "/api/v1/applications",
		Error:    "execute: connection refused",
		Duration: 2 * time.Second,
	})

	entry := parseLogEntry(t, &buf)

	if entry["error"] != "execute: connection refused" {
		t.Errorf("expected error message, got %v", entry["error"])
	}
	if entry["level"] != "ERROR" {
		t.Errorf("expected level=ERROR, got %v", entry["level"])
	}
	// Error entries should not have a status code.
	if _, ok := entry["status_code"]; ok {
		t.Error("error entries should not have status_code")
	}
}

func TestLogExecute_NoUserFieldWhenEmpty(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.LogExecute(context.Background(), Entry{
		Tool:       "execute_operation",
		Method:     "GET",
		Path:       "/api/v1/applications",
		StatusCode: IntPtr(200),
	})

	entry := parseLogEntry(t, &buf)

	if _, ok := entry["user"]; ok {
		t.Error("user field should be absent when empty")
	}
}

func TestLogSearch_BasicFields(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.LogSearch(context.Background(), Entry{
		Tool:        "search_operations",
		Query:       "sync application",
		ResultCount: 5,
		Duration:    10 * time.Millisecond,
	})

	entry := parseLogEntry(t, &buf)

	if entry["tool"] != "search_operations" {
		t.Errorf("expected tool=search_operations, got %v", entry["tool"])
	}
	if entry["query"] != "sync application" {
		t.Errorf("expected query='sync application', got %v", entry["query"])
	}
	if entry["result_count"] != float64(5) {
		t.Errorf("expected result_count=5, got %v", entry["result_count"])
	}
	// No user field when empty.
	if _, ok := entry["user"]; ok {
		t.Error("user field should be absent when empty")
	}
}

func TestLogSearch_WithUser(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.LogSearch(context.Background(), Entry{
		Tool:        "search_operations",
		User:        "bob@example.com",
		Query:       "list projects",
		ResultCount: 3,
		Duration:    5 * time.Millisecond,
	})

	entry := parseLogEntry(t, &buf)

	if entry["user"] != "bob@example.com" {
		t.Errorf("expected user=bob@example.com, got %v", entry["user"])
	}
}

func TestLogSearch_WithError(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.LogSearch(context.Background(), Entry{
		Tool:  "search_operations",
		Query: "sync",
		Error: "search failed: index not ready",
	})

	entry := parseLogEntry(t, &buf)

	if entry["error"] != "search failed: index not ready" {
		t.Errorf("expected error, got %v", entry["error"])
	}
	if entry["level"] != "ERROR" {
		t.Errorf("expected level=ERROR, got %v", entry["level"])
	}
}
