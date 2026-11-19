package openapi

import (
	"math"
	"slices"
	"sort"
	"strings"
	"unicode"
)

const defaultMaxResults = 20

const (
	scoreNameExact   = 10
	scoreNamePartial = 5
	scoreSummary     = 3
	scoreParameter   = 2
	scoreDescription = 1
)

// Search ranks endpoints against a keyword query. An operationId or a path
// returns that operation; otherwise every term scores on its own, the
// operation name (operationId and path segments) weighing most, rare terms
// weighing more than common ones ("get logs": "logs" decides), and a
// "+term" is required.
func Search(endpoints []Endpoint, query string, maxResults int) []Endpoint {
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return head(endpoints, maxResults)
	}
	if exact := exactMatches(endpoints, query); len(exact) > 0 {
		return head(exact, maxResults)
	}

	var terms, required []string
	for field := range strings.FieldsSeq(query) {
		ws := words(strings.TrimPrefix(field, "+"))
		terms = append(terms, ws...)
		if strings.HasPrefix(field, "+") {
			required = append(required, ws...)
		}
	}

	docs := make([]searchDoc, len(endpoints))
	for i, ep := range endpoints {
		docs[i] = newSearchDoc(ep)
	}
	weights := make([]float64, len(terms))
	for i, t := range terms {
		matching := 0
		for _, d := range docs {
			if d.score(t) > 0 {
				matching++
			}
		}
		if matching > 0 {
			weights[i] = math.Log(1 + float64(len(docs))/float64(matching))
		}
	}

	type scored struct {
		endpoint Endpoint
		score    float64
		nameLen  int
	}
	results := make([]scored, 0, len(endpoints))
	for i, d := range docs {
		if slices.ContainsFunc(required, func(t string) bool { return d.score(t) == 0 }) {
			continue
		}
		var s float64
		for j, t := range terms {
			s += float64(d.score(t)) * weights[j]
		}
		if s > 0 {
			results = append(results, scored{endpoint: endpoints[i], score: s, nameLen: len(d.operation)})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		if results[i].nameLen != results[j].nameLen {
			return results[i].nameLen < results[j].nameLen
		}
		return results[i].endpoint.OperationID < results[j].endpoint.OperationID
	})

	out := make([]Endpoint, 0, min(len(results), maxResults))
	for _, r := range head(results, maxResults) {
		out = append(out, r.endpoint)
	}
	return out
}

func head[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func exactMatches(endpoints []Endpoint, query string) []Endpoint {
	var out []Endpoint
	for _, ep := range endpoints {
		if strings.EqualFold(ep.OperationID, query) || ep.Path == query {
			out = append(out, ep)
		}
	}
	return out
}

type searchDoc struct {
	operation   []string
	name        []string
	summary     []string
	parameters  []string
	description []string
}

func newSearchDoc(ep Endpoint) searchDoc {
	operation := slices.DeleteFunc(words(ep.OperationID), func(w string) bool { return w == "service" })

	name := slices.Clone(operation)
	for seg := range strings.SplitSeq(ep.Path, "/") {
		if seg != "" && !strings.HasPrefix(seg, "{") && seg != "api" && seg != "v1" {
			name = append(name, words(seg)...)
		}
	}

	var params []string
	for _, p := range ep.Parameters {
		params = append(params, words(p.Name)...)
	}
	for _, p := range ep.BodyProperties {
		params = append(params, words(p.Name)...)
	}

	return searchDoc{
		operation:   operation,
		name:        name,
		summary:     words(ep.Summary),
		parameters:  params,
		description: words(ep.Description),
	}
}

func (d searchDoc) score(term string) int {
	s := 0
	switch {
	case slices.Contains(d.name, term):
		s += scoreNameExact
	case len(term) >= 3 && slices.ContainsFunc(d.name, func(n string) bool { return strings.Contains(n, term) }):
		s += scoreNamePartial
	}
	if slices.Contains(d.summary, term) {
		s += scoreSummary
	}
	if slices.Contains(d.parameters, term) {
		s += scoreParameter
	}
	if slices.Contains(d.description, term) {
		s += scoreDescription
	}
	return s
}

// words splits text into lowercase, singular word tokens, breaking
// camelCase and snake_case (ApplicationService_PodLogs -> application,
// service, pod, log).
func words(text string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, singular(strings.ToLower(string(cur))))
			cur = cur[:0]
		}
	}
	runes := []rune(text)
	for i, r := range runes {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
			continue
		case unicode.IsUpper(r) && i > 0 && len(cur) > 0 &&
			(unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))):
			flush()
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

func singular(w string) string {
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		return w[:len(w)-1]
	}
	return w
}
