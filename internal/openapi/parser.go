package openapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ParseSpec extracts Endpoint definitions from a raw Swagger 2.0 JSON spec.
// ArgoCD uses Swagger 2.0, so this parser is purpose-built for that format.
func ParseSpec(raw json.RawMessage) ([]Endpoint, error) {
	var doc struct {
		Paths       map[string]map[string]json.RawMessage `json:"paths"`
		Definitions map[string]json.RawMessage             `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse openapi doc: %w", err)
	}

	var endpoints []Endpoint
	for path, methods := range doc.Paths {
		for method, opRaw := range methods {
			method = strings.ToUpper(method)
			if !isHTTPMethod(method) {
				continue
			}
			ep, err := parseOperation(opRaw, method, path, doc.Definitions)
			if err != nil {
				continue
			}
			endpoints = append(endpoints, ep)
		}
	}

	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Path < endpoints[j].Path
	})
	return endpoints, nil
}

func parseOperation(raw json.RawMessage, method, path string, definitions map[string]json.RawMessage) (Endpoint, error) {
	var op struct {
		Summary     string   `json:"summary"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		Parameters  []struct {
			Name        string `json:"name"`
			In          string `json:"in"`
			Required    bool   `json:"required"`
			Description string `json:"description"`
			Type        string `json:"type"`
			Schema      *struct {
				Ref string `json:"$ref"`
			} `json:"schema"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &op); err != nil {
		return Endpoint{}, err
	}

	params := make([]Parameter, 0, len(op.Parameters))
	var requestBody string

	for _, p := range op.Parameters {
		if p.In == "body" && p.Schema != nil && p.Schema.Ref != "" {
			requestBody = resolveRefSummary(p.Schema.Ref, definitions)
			continue
		}
		params = append(params, Parameter{
			Name:        p.Name,
			In:          p.In,
			Required:    p.Required,
			Description: p.Description,
			Type:        p.Type,
		})
	}

	return Endpoint{
		Method:      method,
		Path:        path,
		Summary:     op.Summary,
		Description: truncate(op.Description, 300),
		Tags:        op.Tags,
		Parameters:  params,
		RequestBody: requestBody,
	}, nil
}

// resolveRefSummary extracts property names from a $ref definition to give the
// LLM a lightweight schema preview.
func resolveRefSummary(ref string, definitions map[string]json.RawMessage) string {
	parts := strings.Split(ref, "/")
	name := parts[len(parts)-1]

	raw, ok := definitions[name]
	if !ok {
		return name
	}

	var schema struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return name
	}

	fields := make([]string, 0, len(schema.Properties))
	for k, v := range schema.Properties {
		if v.Type != "" {
			fields = append(fields, k+":"+v.Type)
		} else {
			fields = append(fields, k)
		}
	}
	sort.Strings(fields)
	return name + "{" + strings.Join(fields, ", ") + "}"
}

func isHTTPMethod(m string) bool {
	switch m {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	}
	return false
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
