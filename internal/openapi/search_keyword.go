package openapi

import "context"

// KeywordSearcher wraps the keyword search function to implement the Searcher interface.
type KeywordSearcher struct {
	endpoints []Endpoint
}

// NewKeywordSearcher creates a keyword-based searcher.
func NewKeywordSearcher(endpoints []Endpoint) *KeywordSearcher {
	return &KeywordSearcher{endpoints: endpoints}
}

// Search performs keyword matching on the endpoint list.
func (k *KeywordSearcher) Search(_ context.Context, query string, maxResults int) ([]Endpoint, error) {
	return Search(k.endpoints, query, maxResults), nil
}
