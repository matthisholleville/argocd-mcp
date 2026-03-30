package openapi

// Endpoint represents a single API operation extracted from the ArgoCD OpenAPI spec.
type Endpoint struct {
	Method         string         `json:"method"`
	Path           string         `json:"path"`
	Summary        string         `json:"summary"`
	Description    string         `json:"description,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	Parameters     []Parameter    `json:"parameters,omitempty"`
	RequestBody    string         `json:"request_body,omitempty"`
	OperationID    string         `json:"operation_id,omitempty"`
	BodyProperties []BodyProperty `json:"body_properties,omitempty"`
}

// Parameter describes one API parameter (path, query, header).
type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

// BodyProperty describes a top-level property from a request body schema.
// Used in generated tools mode to expose body fields as individual tool parameters.
type BodyProperty struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}
