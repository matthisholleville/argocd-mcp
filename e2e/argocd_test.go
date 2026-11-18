//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/matthisholleville/argocd-mcp/e2e/fixtures"
)

var protocolVersions = []string{
	mcp.ProtocolVersion20251125,
	mcp.ProtocolVersion20260728,
}

func TestSearchMode(t *testing.T) {
	argo := fixtures.RunArgoCD(t)
	env := serverEnv(argo)
	endpoint := startHTTPServer(t, env)

	for _, version := range protocolVersions {
		t.Run("http/"+version, func(t *testing.T) {
			assertSearchFlow(t, connectHTTP(t, endpoint, version))
		})
		t.Run("stdio/"+version, func(t *testing.T) {
			assertSearchFlow(t, connectStdio(t, env, version))
		})
	}
}

func assertSearchFlow(t *testing.T, c *client.Client) {
	t.Helper()

	tools := listTools(t, c)
	search, ok := tools["search_operations"]
	if !ok {
		t.Fatalf("search_operations not listed, got %v", keys(tools))
	}
	if _, ok := tools["execute_operation"]; !ok {
		t.Fatalf("execute_operation not listed, got %v", keys(tools))
	}
	if a := search.Annotations; !isTrue(a.ReadOnlyHint) || !isFalse(a.DestructiveHint) {
		t.Errorf("search_operations readOnlyHint=%s destructiveHint=%s, want true/false", show(a.ReadOnlyHint), show(a.DestructiveHint))
	}

	res := callTool(t, c, "search_operations", map[string]any{"query": "list applications"})
	if res.IsError || !strings.Contains(resultText(res), "/api/v1/applications") {
		t.Fatalf("search did not find the applications endpoint: %s", resultText(res))
	}

	res = callTool(t, c, "execute_operation", map[string]any{"method": "GET", "path": "/api/v1/applications"})
	if res.IsError || !strings.Contains(resultText(res), fixtures.DemoApp) {
		t.Fatalf("listing applications did not return %q: %s", fixtures.DemoApp, resultText(res))
	}
}

func TestDisableWriteBlocksDelete(t *testing.T) {
	argo := fixtures.RunArgoCD(t)
	endpoint := startHTTPServer(t, serverEnv(argo, "DISABLE_WRITE=true"))
	c := connectHTTP(t, endpoint, mcp.LATEST_PROTOCOL_VERSION)

	res := callTool(t, c, "execute_operation", map[string]any{
		"method": "DELETE",
		"path":   "/api/v1/applications/" + fixtures.DemoApp,
	})
	if !res.IsError {
		t.Fatalf("DELETE was not blocked: %s", resultText(res))
	}

	res = callTool(t, c, "execute_operation", map[string]any{
		"method": "GET",
		"path":   "/api/v1/applications/" + fixtures.DemoApp,
	})
	if res.IsError {
		t.Fatalf("application gone after blocked DELETE: %s", resultText(res))
	}
}

func TestGeneratedMode(t *testing.T) {
	argo := fixtures.RunArgoCD(t)
	endpoint := startHTTPServer(t, serverEnv(argo, "TOOL_MODE=generated"))
	c := connectHTTP(t, endpoint, mcp.LATEST_PROTOCOL_VERSION)

	tools := listTools(t, c)
	list, ok := tools["argocd_application_list"]
	if !ok {
		t.Fatalf("argocd_application_list not generated (%d tools)", len(tools))
	}
	if a := list.Annotations; !isTrue(a.ReadOnlyHint) || !isFalse(a.DestructiveHint) {
		t.Errorf("argocd_application_list readOnlyHint=%s destructiveHint=%s, want true/false", show(a.ReadOnlyHint), show(a.DestructiveHint))
	}

	res := callTool(t, c, "argocd_application_list", map[string]any{})
	if res.IsError || !strings.Contains(resultText(res), fixtures.DemoApp) {
		t.Fatalf("argocd_application_list did not return %q: %s", fixtures.DemoApp, resultText(res))
	}
}

func TestVectorSearch(t *testing.T) {
	argo := fixtures.RunArgoCD(t)
	ollama := fixtures.RunOllama(t)
	endpoint := startHTTPServer(t, serverEnv(argo,
		"EMBEDDINGS_ENABLED=true",
		"OLLAMA_URL="+ollama.URL,
		"EMBEDDINGS_MODEL="+fixtures.EmbeddingsModel,
	))
	c := connectHTTP(t, endpoint, mcp.LATEST_PROTOCOL_VERSION)

	res := callTool(t, c, "search_operations", map[string]any{"query": "uninstall software"})
	if res.IsError {
		t.Fatalf("search failed: %s", resultText(res))
	}
	var hits []struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.Unmarshal([]byte(resultText(res)), &hits); err != nil {
		t.Fatalf("decode search results: %v: %s", err, resultText(res))
	}
	top := make([]string, 0, 3)
	for _, h := range hits[:min(len(hits), 3)] {
		top = append(top, h.OperationID)
	}
	if !slices.Contains(top, "ApplicationService_Delete") {
		t.Fatalf("semantic query did not rank ApplicationService_Delete in the top 3, got %v", top)
	}
}

func TestTLSVerification(t *testing.T) {
	argo := fixtures.RunArgoCD(t)
	upstream, err := url.Parse(argo.URL)
	if err != nil {
		t.Fatal(err)
	}
	tlsArgo := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
	t.Cleanup(tlsArgo.Close)

	t.Run("self-signed certificate is rejected by default", func(t *testing.T) {
		out := runServerExpectingFailure(t, serverEnv(argo, "ARGOCD_BASE_URL="+tlsArgo.URL))
		if !strings.Contains(out, "certificate") {
			t.Fatalf("startup did not fail on certificate verification:\n%s", out)
		}
	})

	t.Run("ARGOCD_TLS_INSECURE skips verification", func(t *testing.T) {
		endpoint := startHTTPServer(t, serverEnv(argo, "ARGOCD_BASE_URL="+tlsArgo.URL, "ARGOCD_TLS_INSECURE=true"))
		c := connectHTTP(t, endpoint, mcp.LATEST_PROTOCOL_VERSION)

		res := callTool(t, c, "execute_operation", map[string]any{"method": "GET", "path": "/api/v1/applications"})
		if res.IsError || !strings.Contains(resultText(res), fixtures.DemoApp) {
			t.Fatalf("listing applications through TLS did not return %q: %s", fixtures.DemoApp, resultText(res))
		}
	})
}

func listTools(t *testing.T, c *client.Client) map[string]mcp.Tool {
	t.Helper()
	res, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	tools := make(map[string]mcp.Tool, len(res.Tools))
	for _, tool := range res.Tools {
		tools[tool.Name] = tool
	}
	return tools
}

func keys(m map[string]mcp.Tool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func isTrue(b *bool) bool  { return b != nil && *b }
func isFalse(b *bool) bool { return b != nil && !*b }

func show(b *bool) string {
	if b == nil {
		return "unset"
	}
	return strconv.FormatBool(*b)
}
