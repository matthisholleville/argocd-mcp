//go:build e2e

package fixtures

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	ollamaImage = "ollama/ollama:0.40.14"
	ollamaPort  = "11434/tcp"

	EmbeddingsModel = "nomic-embed-text"
)

type Ollama struct {
	URL string
}

var (
	ollamaMu        sync.Mutex
	ollamaShared    *Ollama
	ollamaContainer *testcontainers.DockerContainer
)

func RunOllama(t testing.TB) *Ollama {
	t.Helper()
	ollamaMu.Lock()
	defer ollamaMu.Unlock()

	if ollamaShared != nil {
		return ollamaShared
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	o, err := bootstrapOllama(ctx, t)
	if err != nil {
		t.Fatalf("start ollama: %v", err)
	}
	ollamaShared = o
	return ollamaShared
}

func CleanupOllama() {
	if ollamaContainer != nil {
		_ = testcontainers.TerminateContainer(ollamaContainer)
	}
}

func bootstrapOllama(ctx context.Context, t testing.TB) (*Ollama, error) {
	t.Logf("starting %s", ollamaImage)
	ctr, err := testcontainers.Run(ctx, ollamaImage,
		testcontainers.WithExposedPorts(ollamaPort),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/api/tags").WithPort(ollamaPort)),
	)
	ollamaContainer = ctr
	if err != nil {
		return nil, fmt.Errorf("run ollama: %w", err)
	}

	start := time.Now()
	code, reader, err := ctr.Exec(ctx, []string{"ollama", "pull", EmbeddingsModel}, tcexec.Multiplexed())
	if err != nil {
		return nil, fmt.Errorf("pull %s: %w", EmbeddingsModel, err)
	}
	out, _ := io.ReadAll(reader)
	if code != 0 {
		return nil, fmt.Errorf("pull %s: exit %d: %s", EmbeddingsModel, code, out)
	}
	t.Logf("pulled %s (%s)", EmbeddingsModel, time.Since(start).Round(time.Second))

	endpoint, err := ctr.PortEndpoint(ctx, ollamaPort, "http")
	if err != nil {
		return nil, fmt.Errorf("ollama endpoint: %w", err)
	}
	return &Ollama{URL: endpoint + "/api"}, nil
}
