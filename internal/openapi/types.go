package openapi

// Endpoint represents a single API operation extracted from the ArgoCD OpenAPI spec.
type Endpoint struct {
	Method      string      `json:"method"`
	Path        string      `json:"path"`
	Summary     string      `json:"summary"`
	Description string      `json:"description,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Parameters  []Parameter `json:"parameters,omitempty"`
	RequestBody string      `json:"request_body,omitempty"`
}

// Parameter describes one API parameter (path, query, header).
type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}
