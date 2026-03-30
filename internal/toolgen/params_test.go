package toolgen

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/matthisholleville/argocd-mcp/internal/openapi"
)

// getProp extracts a property from a tool's input schema as map[string]any.
func getProp(t *testing.T, tool mcp.Tool, name string) map[string]any {
	t.Helper()
	raw, ok := tool.InputSchema.Properties[name]
	if !ok {
		t.Fatalf("expected %q property in schema", name)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("property %q is not map[string]any, got %T", name, raw)
	}
	return m
}

func TestBuildToolOptions_PathParams(t *testing.T) {
	ep := openapi.Endpoint{
		Parameters: []openapi.Parameter{
			{Name: "name", In: "path", Required: true, Type: "string", Description: "App name"},
		},
	}

	opts := BuildToolOptions(ep)
	tool := mcp.NewTool("test", opts...)

	prop := getProp(t, tool, "name")
	if prop["type"] != "string" {
		t.Errorf("expected type=string, got %v", prop["type"])
	}
	if prop["description"] != "App name" {
		t.Errorf("expected description='App name', got %v", prop["description"])
	}
	if !contains(tool.InputSchema.Required, "name") {
		t.Errorf("expected 'name' in required, got %v", tool.InputSchema.Required)
	}
}

func TestBuildToolOptions_QueryParams(t *testing.T) {
	ep := openapi.Endpoint{
		Parameters: []openapi.Parameter{
			{Name: "refresh", In: "query", Required: false, Type: "string"},
			{Name: "appNamespace", In: "query", Required: false, Type: "string"},
		},
	}

	opts := BuildToolOptions(ep)
	tool := mcp.NewTool("test", opts...)

	getProp(t, tool, "refresh")
	getProp(t, tool, "appNamespace")

	// Query params are optional — should not be in required.
	for _, r := range tool.InputSchema.Required {
		if r == "refresh" || r == "appNamespace" {
			t.Errorf("query param %q should not be required", r)
		}
	}
}

func TestBuildToolOptions_BodyFlatten_Primitives(t *testing.T) {
	ep := openapi.Endpoint{
		BodyProperties: []openapi.BodyProperty{
			{Name: "revision", Type: "string", Description: "Target revision"},
			{Name: "dryRun", Type: "boolean", Description: "Simulate sync"},
			{Name: "retryCount", Type: "integer", Description: "Retry count"},
		},
	}

	opts := BuildToolOptions(ep)
	tool := mcp.NewTool("test", opts...)

	if prop := getProp(t, tool, "revision"); prop["type"] != "string" {
		t.Errorf("revision type = %v, want string", prop["type"])
	}
	if prop := getProp(t, tool, "dryRun"); prop["type"] != "boolean" {
		t.Errorf("dryRun type = %v, want boolean", prop["type"])
	}
	if prop := getProp(t, tool, "retryCount"); prop["type"] != "number" {
		t.Errorf("retryCount type = %v, want number", prop["type"])
	}
}

func TestBuildToolOptions_BodyFlatten_NestedObject(t *testing.T) {
	ep := openapi.Endpoint{
		BodyProperties: []openapi.BodyProperty{
			{Name: "strategy", Type: "object", Description: "Sync strategy"},
		},
	}

	opts := BuildToolOptions(ep)
	tool := mcp.NewTool("test", opts...)

	prop := getProp(t, tool, "strategy")
	if prop["type"] != "string" {
		t.Errorf("strategy type = %v, want string (JSON object)", prop["type"])
	}
	desc, _ := prop["description"].(string)
	if desc != "Sync strategy (JSON object)" {
		t.Errorf("strategy description = %q, want 'Sync strategy (JSON object)'", desc)
	}
}

func TestBuildToolOptions_BodyFlatten_Array(t *testing.T) {
	ep := openapi.Endpoint{
		BodyProperties: []openapi.BodyProperty{
			{Name: "resources", Type: "array", Description: "Resources to sync"},
		},
	}

	opts := BuildToolOptions(ep)
	tool := mcp.NewTool("test", opts...)

	prop := getProp(t, tool, "resources")
	if prop["type"] != "string" {
		t.Errorf("resources type = %v, want string (JSON array)", prop["type"])
	}
	desc, _ := prop["description"].(string)
	if desc != "Resources to sync (JSON array)" {
		t.Errorf("resources description = %q", desc)
	}
}

func TestBuildToolOptions_BodyRequired(t *testing.T) {
	ep := openapi.Endpoint{
		BodyProperties: []openapi.BodyProperty{
			{Name: "revision", Type: "string", Required: true},
			{Name: "dryRun", Type: "boolean", Required: false},
		},
	}

	opts := BuildToolOptions(ep)
	tool := mcp.NewTool("test", opts...)

	if !contains(tool.InputSchema.Required, "revision") {
		t.Errorf("expected 'revision' in required, got %v", tool.InputSchema.Required)
	}
	if contains(tool.InputSchema.Required, "dryRun") {
		t.Error("dryRun should not be required")
	}
}

func TestBuildToolOptions_HeaderParamsIgnored(t *testing.T) {
	ep := openapi.Endpoint{
		Parameters: []openapi.Parameter{
			{Name: "Authorization", In: "header", Type: "string"},
			{Name: "name", In: "path", Required: true, Type: "string"},
		},
	}

	opts := BuildToolOptions(ep)
	tool := mcp.NewTool("test", opts...)

	if _, ok := tool.InputSchema.Properties["Authorization"]; ok {
		t.Error("header params should be excluded")
	}
	getProp(t, tool, "name")
}

func TestBuildToolOptions_MixedPathQueryBody(t *testing.T) {
	ep := openapi.Endpoint{
		Parameters: []openapi.Parameter{
			{Name: "name", In: "path", Required: true, Type: "string"},
			{Name: "appNamespace", In: "query", Type: "string"},
		},
		BodyProperties: []openapi.BodyProperty{
			{Name: "revision", Type: "string", Required: true},
			{Name: "prune", Type: "boolean"},
		},
	}

	opts := BuildToolOptions(ep)
	tool := mcp.NewTool("test", opts...)

	for _, name := range []string{"name", "appNamespace", "revision", "prune"} {
		getProp(t, tool, name)
	}

	if !contains(tool.InputSchema.Required, "name") {
		t.Error("name should be required")
	}
	if !contains(tool.InputSchema.Required, "revision") {
		t.Error("revision should be required")
	}
	if contains(tool.InputSchema.Required, "appNamespace") {
		t.Error("appNamespace should not be required")
	}
	if contains(tool.InputSchema.Required, "prune") {
		t.Error("prune should not be required")
	}
}

func TestBuildToolOptions_Empty(t *testing.T) {
	ep := openapi.Endpoint{}
	opts := BuildToolOptions(ep)
	if len(opts) != 0 {
		t.Errorf("expected 0 options for empty endpoint, got %d", len(opts))
	}
}

func TestBuildToolOptions_BooleanQueryParam(t *testing.T) {
	ep := openapi.Endpoint{
		Parameters: []openapi.Parameter{
			{Name: "force", In: "query", Type: "boolean"},
		},
	}

	opts := BuildToolOptions(ep)
	tool := mcp.NewTool("test", opts...)

	prop := getProp(t, tool, "force")
	if prop["type"] != "boolean" {
		t.Errorf("force type = %v, want boolean", prop["type"])
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
