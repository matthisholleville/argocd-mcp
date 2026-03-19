package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestHandleExecute_DisableWriteBlocksWriteMethods(t *testing.T) {
	writeMethods := []string{"POST", "PUT", "PATCH", "DELETE"}
	gw := newTestGateway(t)
	handler := handleExecute(gw, true)

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
	handler := handleExecute(gw, true)

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
	handler := handleExecute(gw, false)

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
