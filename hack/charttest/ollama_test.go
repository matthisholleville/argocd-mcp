//go:build chart

package charttest

import (
	"os"
	"os/exec"
	"regexp"
	"testing"
)

func TestE2EOllamaImageMatchesChart(t *testing.T) {
	src, err := os.ReadFile("../../e2e/fixtures/ollama.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`ollamaImage\s*=\s*"([^"]+)"`).FindSubmatch(src)
	if m == nil {
		t.Fatal("ollamaImage not found in e2e/fixtures/ollama.go")
	}

	out, err := exec.Command("helm", "template", "argocd-mcp", "../../charts/argocd-mcp", "--namespace", "argocd-mcp",
		"--set", "argocd.baseURL=https://argocd.example.com", "--set", "argocd.token=test",
		"--set", "embeddings.enabled=true").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	c := regexp.MustCompile(`image:\s*"?(ollama/ollama:[^"\s]+)`).FindSubmatch(out)
	if c == nil {
		t.Fatal("no ollama image rendered with embeddings.enabled=true")
	}

	if string(m[1]) != string(c[1]) {
		t.Errorf("e2e tests %s but the chart ships %s: set ollamaImage in e2e/fixtures/ollama.go to %q", m[1], c[1], c[1])
	}
}
