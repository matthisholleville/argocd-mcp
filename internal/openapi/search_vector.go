package openapi

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	chromem "github.com/philippgille/chromem-go"
)

const collectionName = "argocd-endpoints"

// VectorIndex holds an in-memory vector index of ArgoCD endpoints.
type VectorIndex struct {
	collection *chromem.Collection
	endpoints  map[string]Endpoint // keyed by ID (method:path)
	topK       int
}

// VectorConfig holds settings for the vector search backend.
type VectorConfig struct {
	OllamaURL string // e.g. "http://localhost:11434/api"
	Model     string // e.g. "nomic-embed-text"
	TopK      int
}

// NewVectorIndex creates a vector index backed by Ollama embeddings.
func NewVectorIndex(cfg VectorConfig) (*VectorIndex, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("vector search: Model must be set")
	}
	if cfg.TopK <= 0 {
		cfg.TopK = 10
	}

	embeddingFunc := chromem.NewEmbeddingFuncOllama(cfg.Model, cfg.OllamaURL)
	db := chromem.NewDB()
	col, err := db.CreateCollection(collectionName, nil, embeddingFunc)
	if err != nil {
		return nil, fmt.Errorf("vector search: create collection: %w", err)
	}

	return &VectorIndex{
		collection: col,
		endpoints:  make(map[string]Endpoint),
		topK:       cfg.TopK,
	}, nil
}

// Index embeds all endpoints into the vector store via Ollama.
func (v *VectorIndex) Index(ctx context.Context, endpoints []Endpoint) error {
	docs := make([]chromem.Document, 0, len(endpoints))

	for _, ep := range endpoints {
		id := ep.Method + ":" + ep.Path
		v.endpoints[id] = ep

		docs = append(docs, chromem.Document{
			ID:      id,
			Content: buildEmbeddingText(ep),
		})
	}

	if err := v.collection.AddDocuments(ctx, docs, runtime.NumCPU()); err != nil {
		return fmt.Errorf("vector search: index endpoints: %w", err)
	}

	return nil
}

// Search performs semantic search over the indexed endpoints.
func (v *VectorIndex) Search(ctx context.Context, query string, maxResults int) ([]Endpoint, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return v.listAll(maxResults), nil
	}

	nResults := min(maxResults, v.collection.Count())
	if nResults == 0 {
		return nil, nil
	}

	results, err := v.collection.Query(ctx, query, nResults, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("vector search: query: %w", err)
	}

	out := make([]Endpoint, 0, len(results))
	for _, r := range results {
		if ep, ok := v.endpoints[r.ID]; ok {
			out = append(out, ep)
		}
	}
	return out, nil
}

func (v *VectorIndex) listAll(maxResults int) []Endpoint {
	out := make([]Endpoint, 0, len(v.endpoints))
	for _, ep := range v.endpoints {
		out = append(out, ep)
		if len(out) >= maxResults {
			break
		}
	}
	return out
}

func buildEmbeddingText(ep Endpoint) string {
	var sb strings.Builder
	sb.WriteString(ep.Method)
	sb.WriteString(" ")
	sb.WriteString(ep.Path)
	sb.WriteString(" ")
	sb.WriteString(ep.Summary)
	sb.WriteString(" ")
	sb.WriteString(ep.Description)
	for _, t := range ep.Tags {
		sb.WriteString(" ")
		sb.WriteString(t)
	}
	for _, p := range ep.Parameters {
		sb.WriteString(" ")
		sb.WriteString(p.Name)
		sb.WriteString(" ")
		sb.WriteString(p.Description)
	}
	return sb.String()
}
