//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/matthisholleville/argocd-mcp/e2e/fixtures"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "argocd-mcp-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "argocd-mcp")
	build := exec.Command("go", "build", "-o", binary, "../cmd/argocd-mcp")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build argocd-mcp:", err)
		os.Exit(1)
	}

	code := m.Run()

	fixtures.CleanupArgoCD()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func serverEnv(argo *fixtures.ArgoCD, extra ...string) []string {
	return append([]string{
		"ARGOCD_BASE_URL=" + argo.URL,
		"ARGOCD_TOKEN=" + argo.Token,
		"AUDIT_LOG=false",
	}, extra...)
}

func startHTTPServer(t *testing.T, env []string) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	var logs strings.Builder
	cmd := exec.Command(binary)
	cmd.Env = append(env, "MCP_TRANSPORT=http", "MCP_ADDR="+addr)
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("argocd-mcp logs:\n%s", logs.String())
		}
	})

	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			_ = conn.Close()
			return "http://" + addr + "/mcp"
		}
		if time.Now().After(deadline) {
			t.Fatalf("argocd-mcp did not listen on %s:\n%s", addr, logs.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func connect(t *testing.T, tr transport.Interface, protocolVersion string) *client.Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c := client.NewClient(tr, client.WithProtocolVersion(protocolVersion))
	t.Cleanup(func() { _ = c.Close() })
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("start client: %v", err)
	}
	req := mcp.InitializeRequest{}
	req.Params.ClientInfo = mcp.Implementation{Name: "argocd-mcp-e2e", Version: "test"}
	if _, err := c.Initialize(ctx, req); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if got := c.ProtocolVersion(); got != protocolVersion {
		t.Fatalf("negotiated protocol %q, want %q", got, protocolVersion)
	}
	return c
}

func connectHTTP(t *testing.T, endpoint, protocolVersion string) *client.Client {
	t.Helper()
	tr, err := transport.NewStreamableHTTP(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	return connect(t, tr, protocolVersion)
}

func connectStdio(t *testing.T, env []string, protocolVersion string) *client.Client {
	t.Helper()
	return connect(t, transport.NewStdio(binary, env), protocolVersion)
}

func callTool(t *testing.T, c *client.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return res
}

func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
