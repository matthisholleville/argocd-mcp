package openapi

import (
	"encoding/json"
	"reflect"
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

func TestFilterReadOnly(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/applications", Summary: "List applications"},
		{Method: "POST", Path: "/api/v1/applications", Summary: "Create application"},
		{Method: "PUT", Path: "/api/v1/applications/{name}", Summary: "Update application"},
		{Method: "PATCH", Path: "/api/v1/applications/{name}", Summary: "Patch application"},
		{Method: "DELETE", Path: "/api/v1/applications/{name}", Summary: "Delete application"},
		{Method: "HEAD", Path: "/api/v1/applications", Summary: "Head applications"},
	}

	filtered := FilterReadOnly(endpoints)

	// Only GET and HEAD should remain.
	if len(filtered) != 2 {
		t.Fatalf("expected 2 read-only endpoints, got %d", len(filtered))
	}
	for _, ep := range filtered {
		if ep.Method != "GET" && ep.Method != "HEAD" {
			t.Errorf("unexpected write method in results: %s", ep.Method)
		}
	}
}

func TestFilterReadOnly_EmptyInput(t *testing.T) {
	filtered := FilterReadOnly(nil)
	if len(filtered) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(filtered))
	}
}

func TestFilterReadOnly_AllReadOnly(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/a", Summary: "A"},
		{Method: "GET", Path: "/b", Summary: "B"},
	}

	filtered := FilterReadOnly(endpoints)
	if len(filtered) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(filtered))
	}
}

func TestFilterReadOnly_DoesNotMutateOriginal(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/a", Summary: "A"},
		{Method: "DELETE", Path: "/b", Summary: "B"},
	}

	first := endpoints[0]
	second := endpoints[1]
	_ = FilterReadOnly(endpoints)

	if len(endpoints) != 2 {
		t.Errorf("original slice length changed: expected 2, got %d", len(endpoints))
	}
	if !reflect.DeepEqual(endpoints[0], first) {
		t.Error("first element was mutated")
	}
	if !reflect.DeepEqual(endpoints[1], second) {
		t.Error("second element was mutated")
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
