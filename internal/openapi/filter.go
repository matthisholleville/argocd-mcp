package openapi

import "strings"

// FilterByTags returns a new slice containing only endpoints that have at least
// one tag matching the provided list. Matching is case-insensitive.
// If tags is nil or empty, all endpoints are returned (no filtering).
func FilterByTags(endpoints []Endpoint, tags []string) []Endpoint {
	if len(tags) == 0 {
		return endpoints
	}

	allowed := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		allowed[strings.ToLower(t)] = struct{}{}
	}

	filtered := make([]Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		for _, tag := range ep.Tags {
			if _, ok := allowed[strings.ToLower(tag)]; ok {
				filtered = append(filtered, ep)
				break
			}
		}
	}
	return filtered
}

// AllowedEndpoints is a set of (method, path-pattern) pairs used to enforce
// execute-time restrictions. When non-nil, only requests matching an entry in
// the set are permitted.
type AllowedEndpoints struct {
	patterns []methodPattern
}

type methodPattern struct {
	method   string
	segments []string
}

// NewAllowedEndpoints builds an AllowedEndpoints set from the given endpoints.
func NewAllowedEndpoints(endpoints []Endpoint) *AllowedEndpoints {
	patterns := make([]methodPattern, len(endpoints))
	for i, ep := range endpoints {
		patterns[i] = methodPattern{
			method:   strings.ToUpper(ep.Method),
			segments: strings.Split(strings.Trim(ep.Path, "/"), "/"),
		}
	}
	return &AllowedEndpoints{patterns: patterns}
}

// IsAllowed reports whether the given method and path match any endpoint in the
// allowed set. A nil receiver allows everything (no restriction).
func (a *AllowedEndpoints) IsAllowed(method, path string) bool {
	if a == nil {
		return true
	}
	method = strings.ToUpper(method)
	// Strip query string if present (LLM may append ?key=val to the path).
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")

	for _, p := range a.patterns {
		if p.method == method && matchSegments(p.segments, segments) {
			return true
		}
	}
	return false
}

// matchSegments checks whether actual path segments match a pattern.
// Pattern segments wrapped in { } are treated as wildcards matching exactly
// one segment.
func matchSegments(pattern, actual []string) bool {
	if len(pattern) != len(actual) {
		return false
	}
	for i, seg := range pattern {
		if isPathParam(seg) {
			continue
		}
		if seg != actual[i] {
			return false
		}
	}
	return true
}

func isPathParam(seg string) bool {
	return len(seg) >= 2 && seg[0] == '{' && seg[len(seg)-1] == '}'
}
