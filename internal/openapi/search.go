package openapi

import (
	"sort"
	"strings"
)

const defaultMaxResults = 20

// Search performs keyword matching on the endpoint list.
func Search(endpoints []Endpoint, query string, maxResults int) []Endpoint {
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}

	query = strings.ToLower(strings.TrimSpace(query))

	if query == "" {
		if len(endpoints) > maxResults {
			return endpoints[:maxResults]
		}
		return endpoints
	}

	terms := strings.Fields(query)
	type scored struct {
		endpoint Endpoint
		score    int
	}

	results := make([]scored, 0, len(endpoints))
	for _, ep := range endpoints {
		s := scoreEndpoint(ep, terms)
		if s > 0 {
			results = append(results, scored{endpoint: ep, score: s})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	out := make([]Endpoint, len(results))
	for i, r := range results {
		out[i] = r.endpoint
	}
	return out
}

func scoreEndpoint(ep Endpoint, terms []string) int {
	searchText := strings.ToLower(strings.Join([]string{
		ep.Path,
		ep.Summary,
		ep.Description,
		ep.Method,
		strings.Join(ep.Tags, " "),
		paramNames(ep.Parameters),
	}, " "))

	score := 0
	for _, term := range terms {
		score += strings.Count(searchText, term)
	}
	return score
}

func paramNames(params []Parameter) string {
	names := make([]string, len(params))
	for i, p := range params {
		names[i] = p.Name + " " + p.Description
	}
	return strings.Join(names, " ")
}
