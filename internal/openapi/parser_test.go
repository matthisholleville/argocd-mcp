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

// --- FilterByTags tests ---

func TestFilterByTags_MatchesSingleTag(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/version", Tags: []string{"VersionService"}},
		{Method: "GET", Path: "/api/v1/applications", Tags: []string{"ApplicationService"}},
		{Method: "POST", Path: "/api/v1/applications", Tags: []string{"ApplicationService"}},
	}

	filtered := FilterByTags(endpoints, []string{"VersionService"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(filtered))
	}
	if filtered[0].Path != "/api/v1/version" {
		t.Errorf("expected /api/v1/version, got %s", filtered[0].Path)
	}
}

func TestFilterByTags_MatchesMultipleTags(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/version", Tags: []string{"VersionService"}},
		{Method: "GET", Path: "/api/v1/applications", Tags: []string{"ApplicationService"}},
		{Method: "GET", Path: "/api/v1/projects", Tags: []string{"ProjectService"}},
	}

	filtered := FilterByTags(endpoints, []string{"VersionService", "ProjectService"})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(filtered))
	}
}

func TestFilterByTags_NoMatch(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/applications", Tags: []string{"ApplicationService"}},
	}

	filtered := FilterByTags(endpoints, []string{"NonExistentService"})
	if len(filtered) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(filtered))
	}
}

func TestFilterByTags_EmptyTagsReturnsAll(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/a", Tags: []string{"A"}},
		{Method: "GET", Path: "/b", Tags: []string{"B"}},
	}

	filtered := FilterByTags(endpoints, nil)
	if len(filtered) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(filtered))
	}

	filtered2 := FilterByTags(endpoints, []string{})
	if len(filtered2) != 2 {
		t.Errorf("expected 2 endpoints with empty slice, got %d", len(filtered2))
	}
}

func TestFilterByTags_CaseInsensitive(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/version", Tags: []string{"VersionService"}},
	}

	filtered := FilterByTags(endpoints, []string{"versionservice"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 endpoint (case-insensitive), got %d", len(filtered))
	}
}

func TestFilterByTags_DoesNotMutateOriginal(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/a", Tags: []string{"A"}},
		{Method: "GET", Path: "/b", Tags: []string{"B"}},
	}

	original := make([]Endpoint, len(endpoints))
	copy(original, endpoints)
	_ = FilterByTags(endpoints, []string{"A"})

	if !reflect.DeepEqual(endpoints, original) {
		t.Error("original slice was mutated")
	}
}

func TestFilterByTags_EndpointWithMultipleTags(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/multi", Tags: []string{"ServiceA", "ServiceB"}},
		{Method: "GET", Path: "/api/v1/solo", Tags: []string{"ServiceC"}},
	}

	filtered := FilterByTags(endpoints, []string{"ServiceB"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(filtered))
	}
	if filtered[0].Path != "/api/v1/multi" {
		t.Errorf("expected /api/v1/multi, got %s", filtered[0].Path)
	}
}

// --- AllowedEndpoints tests ---

func TestNewAllowedEndpoints_AllowsMatchingEndpoint(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/version"},
		{Method: "GET", Path: "/api/v1/applications"},
	}

	allowed := NewAllowedEndpoints(endpoints)
	if !allowed.IsAllowed("GET", "/api/v1/version") {
		t.Error("expected GET /api/v1/version to be allowed")
	}
	if !allowed.IsAllowed("GET", "/api/v1/applications") {
		t.Error("expected GET /api/v1/applications to be allowed")
	}
}

func TestNewAllowedEndpoints_BlocksUnknownEndpoint(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/version"},
	}

	allowed := NewAllowedEndpoints(endpoints)
	if allowed.IsAllowed("GET", "/api/v1/applications") {
		t.Error("expected GET /api/v1/applications to be blocked")
	}
	if allowed.IsAllowed("POST", "/api/v1/version") {
		t.Error("expected POST /api/v1/version to be blocked (wrong method)")
	}
}

func TestNewAllowedEndpoints_MatchesPathParameters(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/applications/{name}"},
		{Method: "DELETE", Path: "/api/v1/applications/{name}"},
	}

	allowed := NewAllowedEndpoints(endpoints)
	if !allowed.IsAllowed("GET", "/api/v1/applications/myapp") {
		t.Error("expected GET /api/v1/applications/myapp to match {name} pattern")
	}
	if !allowed.IsAllowed("DELETE", "/api/v1/applications/other-app") {
		t.Error("expected DELETE /api/v1/applications/other-app to match")
	}
	if allowed.IsAllowed("GET", "/api/v1/applications/myapp/extra") {
		t.Error("expected extra segments to not match")
	}
}

func TestNewAllowedEndpoints_NestedPathParameters(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/applications/{name}/resource-tree"},
		{Method: "GET", Path: "/api/v1/applications/{name}/logs/{container}"},
	}

	allowed := NewAllowedEndpoints(endpoints)
	if !allowed.IsAllowed("GET", "/api/v1/applications/myapp/resource-tree") {
		t.Error("expected nested path with param to match")
	}
	if !allowed.IsAllowed("GET", "/api/v1/applications/myapp/logs/main") {
		t.Error("expected double param path to match")
	}
	if allowed.IsAllowed("GET", "/api/v1/applications/myapp/sync") {
		t.Error("expected /sync to not match /resource-tree pattern")
	}
}

func TestNewAllowedEndpoints_StripsQueryString(t *testing.T) {
	endpoints := []Endpoint{
		{Method: "GET", Path: "/api/v1/applications"},
	}

	allowed := NewAllowedEndpoints(endpoints)
	if !allowed.IsAllowed("GET", "/api/v1/applications?refresh=true") {
		t.Error("expected path with query string to match")
	}
}

func TestNewAllowedEndpoints_NilIsPermissive(t *testing.T) {
	var allowed *AllowedEndpoints
	if !allowed.IsAllowed("DELETE", "/anything") {
		t.Error("nil AllowedEndpoints should allow everything")
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
