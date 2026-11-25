//go:build e2e

package e2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/matthisholleville/argocd-mcp/e2e/fixtures"
)

func TestServesHealthWhileIndexing(t *testing.T) {
	argo := fixtures.RunArgoCD(t)

	release := make(chan struct{})
	var once sync.Once
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedding":[0.1,0.2,0.3]}`))
	}))
	t.Cleanup(ollama.Close)
	t.Cleanup(func() { once.Do(func() { close(release) }) })

	endpoint := launchHTTPServer(t, serverEnv(argo,
		"EMBEDDINGS_ENABLED=true",
		"OLLAMA_URL="+ollama.URL+"/api",
		"EMBEDDINGS_MODEL=fake",
	))
	base := strings.TrimSuffix(endpoint, "/mcp")

	for path, want := range map[string]int{"/healthz": http.StatusOK, "/readyz": http.StatusServiceUnavailable} {
		if got := statusOf(t, http.MethodGet, base+path); got != want {
			t.Errorf("GET %s while indexing = %d, want %d", path, got, want)
		}
	}
	if got := statusOf(t, http.MethodPost, endpoint); got != http.StatusServiceUnavailable {
		t.Errorf("POST /mcp while indexing = %d, want 503", got)
	}

	once.Do(func() { close(release) })
	deadline := time.Now().Add(time.Minute)
	for statusOf(t, http.MethodGet, base+"/readyz") != http.StatusOK {
		if time.Now().After(deadline) {
			t.Fatal("/readyz never turned 200 after indexing")
		}
		time.Sleep(200 * time.Millisecond)
	}

	c := connectHTTP(t, endpoint, mcp.LATEST_PROTOCOL_VERSION)
	if _, ok := listTools(t, c)["search_operations"]; !ok {
		t.Fatal("search_operations not listed once ready")
	}
}

func statusOf(t *testing.T, method, url string) int {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}
