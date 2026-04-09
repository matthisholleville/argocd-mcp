package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newGatewayWithHandler creates a Gateway backed by a custom HTTP handler.
func newGatewayWithHandler(t *testing.T, handler http.HandlerFunc) *Gateway {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	gw, err := NewGateway(srv.URL, "test-token", false, "", silentLogger())
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	return gw
}

// ArgoCD logs endpoint returns NDJSON (one JSON object per line).
// This must not cause a marshal error.
func TestExecute_NDJSONResponse(t *testing.T) {
	ndjson := "{\"result\":{\"content\":\"line1\"}}\n{\"result\":{\"content\":\"line2\"}}\n"
	gw := newGatewayWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(ndjson))
	})

	result, err := gw.Execute(context.Background(), ExecuteParams{
		Method: "GET",
		Path:   "/api/v1/applications/myapp/pods/pod-1/logs",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var parsed ExecuteResult
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("result should be valid JSON, got: %v", err)
	}
	if parsed.Status != 200 {
		t.Errorf("expected status 200, got %d", parsed.Status)
	}
	// NDJSON body should be wrapped as a JSON string.
	var bodyStr string
	if err := json.Unmarshal(parsed.Body, &bodyStr); err != nil {
		t.Fatalf("expected body to be a JSON string, got: %s", string(parsed.Body))
	}
	if bodyStr != ndjson {
		t.Errorf("expected original NDJSON content preserved, got: %q", bodyStr)
	}
}

// Plain text responses (non-JSON) must also be handled gracefully.
func TestExecute_PlainTextResponse(t *testing.T) {
	gw := newGatewayWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("plain text output"))
	})

	result, err := gw.Execute(context.Background(), ExecuteParams{
		Method: "GET",
		Path:   "/some/endpoint",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var parsed ExecuteResult
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("result should be valid JSON, got: %v", err)
	}
	var bodyStr string
	if err := json.Unmarshal(parsed.Body, &bodyStr); err != nil {
		t.Fatalf("expected body to be a JSON string, got: %s", string(parsed.Body))
	}
	if bodyStr != "plain text output" {
		t.Errorf("expected plain text preserved, got: %q", bodyStr)
	}
}

// Valid JSON responses should be passed through as-is (not double-encoded).
func TestExecute_ValidJSONResponse(t *testing.T) {
	gw := newGatewayWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[{"name":"app1"}]}`))
	})

	result, err := gw.Execute(context.Background(), ExecuteParams{
		Method: "GET",
		Path:   "/api/v1/applications",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var parsed ExecuteResult
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("result should be valid JSON, got: %v", err)
	}
	// Body should be the raw JSON object, not a string.
	var body map[string]any
	if err := json.Unmarshal(parsed.Body, &body); err != nil {
		t.Fatalf("expected body to be a JSON object, got: %s", string(parsed.Body))
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Errorf("expected items array with 1 element, got: %v", body)
	}
}

// Empty response body should produce null, not an empty string.
func TestExecute_EmptyBody(t *testing.T) {
	gw := newGatewayWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	result, err := gw.Execute(context.Background(), ExecuteParams{
		Method: "DELETE",
		Path:   "/api/v1/applications/myapp",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var parsed ExecuteResult
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("result should be valid JSON, got: %v", err)
	}
	if parsed.Status != 204 {
		t.Errorf("expected status 204, got %d", parsed.Status)
	}
	if string(parsed.Body) != "null" {
		t.Errorf("expected null body for empty response, got: %s", string(parsed.Body))
	}
}

// Non-2xx responses with non-JSON body (e.g. HTML error from ingress) must not crash.
func TestExecute_ErrorWithNonJSONBody(t *testing.T) {
	gw := newGatewayWithHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
	})

	result, err := gw.Execute(context.Background(), ExecuteParams{
		Method: "GET",
		Path:   "/api/v1/applications",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var parsed ExecuteResult
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("result should be valid JSON, got: %v", err)
	}
	if parsed.Status != 502 {
		t.Errorf("expected status 502, got %d", parsed.Status)
	}
	// HTML body should be wrapped as a JSON string.
	var bodyStr string
	if err := json.Unmarshal(parsed.Body, &bodyStr); err != nil {
		t.Fatalf("expected body to be a JSON string, got: %s", string(parsed.Body))
	}
	if bodyStr != "<html><body>502 Bad Gateway</body></html>" {
		t.Errorf("expected HTML body preserved, got: %q", bodyStr)
	}
}
