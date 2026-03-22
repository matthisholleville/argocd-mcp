package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleUnhealthyApps_ReturnsPromptMessages(t *testing.T) {
	handler := handleUnhealthyApps()
	result, err := handler(context.Background(), mcp.GetPromptRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "execute_operation") {
		t.Error("expected prompt to reference execute_operation tool")
	}
	if !strings.Contains(text, "Healthy") {
		t.Error("expected prompt to mention Healthy status")
	}
}

func TestHandleAppDiff_UsesAppNameArgument(t *testing.T) {
	handler := handleAppDiff()
	req := mcp.GetPromptRequest{}
	req.Params.Arguments = map[string]string{"appName": "my-app"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "my-app") {
		t.Error("expected prompt to contain the app name")
	}
}

func TestHandleAppDiff_DefaultsWhenNoArgument(t *testing.T) {
	handler := handleAppDiff()
	req := mcp.GetPromptRequest{}
	req.Params.Arguments = map[string]string{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "<appName>") {
		t.Error("expected placeholder when no argument provided")
	}
}

func TestHandleRollback_UsesAppNameArgument(t *testing.T) {
	handler := handleRollback()
	req := mcp.GetPromptRequest{}
	req.Params.Arguments = map[string]string{"appName": "frontend"}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "frontend") {
		t.Error("expected prompt to contain the app name")
	}
}

func TestHandleSyncStatus_ReturnsPromptMessages(t *testing.T) {
	handler := handleSyncStatus()
	result, err := handler(context.Background(), mcp.GetPromptRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "OutOfSync") {
		t.Error("expected prompt to mention OutOfSync")
	}
}

func TestHandleAppLogs_UsesArguments(t *testing.T) {
	handler := handleAppLogs()
	req := mcp.GetPromptRequest{}
	req.Params.Arguments = map[string]string{
		"appName":   "backend",
		"container": "api",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := result.Messages[0].Content.(mcp.TextContent).Text
	if !strings.Contains(text, "backend") {
		t.Error("expected prompt to contain app name")
	}
	if !strings.Contains(text, "api") {
		t.Error("expected prompt to contain container name")
	}
}
