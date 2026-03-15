package openapi

import (
	"encoding/json"
	"testing"
)

func TestParseSpec(t *testing.T) {
	spec := `{
		"swagger": "2.0",
		"paths": {
			"/api/v1/applications": {
				"get": {
					"summary": "List applications",
					"tags": ["ApplicationService"],
					"parameters": [
						{"name": "name", "in": "query", "type": "string", "description": "Filter by name"}
					]
				},
				"post": {
					"summary": "Create application",
					"tags": ["ApplicationService"],
					"parameters": [
						{"name": "body", "in": "body", "schema": {"$ref": "#/definitions/v1alpha1Application"}}
					]
				}
			},
			"/api/v1/applications/{name}": {
				"delete": {
					"summary": "Delete application",
					"tags": ["ApplicationService"],
					"parameters": [
						{"name": "name", "in": "path", "required": true, "type": "string"}
					]
				}
			}
		},
		"definitions": {
			"v1alpha1Application": {
				"properties": {
					"metadata": {"type": "object"},
					"spec": {"type": "object"}
				}
			}
		}
	}`

	endpoints, err := ParseSpec(json.RawMessage(spec))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(endpoints) != 3 {
		t.Fatalf("expected 3 endpoints, got %d", len(endpoints))
	}

	// Verify POST has request body.
	var postApp *Endpoint
	for _, ep := range endpoints {
		if ep.Method == "POST" {
			postApp = &ep
			break
		}
	}
	if postApp == nil {
		t.Fatal("POST endpoint not found")
	}
	if postApp.RequestBody == "" {
		t.Error("expected request body, got empty")
	}
}

func TestSearch_QueryMatch(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/applications", Summary: "List applications"},
		{Method: "POST", Path: "/api/v1/applications/{name}/sync", Summary: "Sync application"},
		{Method: "DELETE", Path: "/api/v1/applications/{name}", Summary: "Delete application"},
	}

	results := Search(endpoints, "sync", 10)
	if len(results) == 0 {
		t.Fatal("expected results for 'sync'")
	}
	if results[0].Summary != "Sync application" {
		t.Errorf("top result = %q, want 'Sync application'", results[0].Summary)
	}
}

func TestSearch_NoMatch(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/applications", Summary: "List applications"},
	}

	results := Search(endpoints, "nonexistent", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/a", Summary: "A"},
		{Method: "GET", Path: "/b", Summary: "B"},
	}

	results := Search(endpoints, "", 10)
	if len(results) != 2 {
		t.Errorf("expected all 2 results, got %d", len(results))
	}
}
