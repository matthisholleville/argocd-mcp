package gateway

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestToolAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		tool        mcp.Tool
		readOnly    bool
		destructive bool
		idempotent  bool
		openWorld   bool
	}{
		{"search", searchTool(ToolOptions{}), true, false, true, false},
		{"search read-only mode", searchTool(ToolOptions{DisableWrite: true}), true, false, true, false},
		{"execute", executeTool(ToolOptions{}), false, true, false, true},
		{"execute read-only mode", executeTool(ToolOptions{DisableWrite: true}), true, false, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.tool.Annotations
			check := func(field string, got *bool, want bool) {
				t.Helper()
				if got == nil || *got != want {
					t.Errorf("%s = %v, want %v", field, got, want)
				}
			}
			check("ReadOnlyHint", a.ReadOnlyHint, tt.readOnly)
			check("DestructiveHint", a.DestructiveHint, tt.destructive)
			check("IdempotentHint", a.IdempotentHint, tt.idempotent)
			check("OpenWorldHint", a.OpenWorldHint, tt.openWorld)
		})
	}
}
