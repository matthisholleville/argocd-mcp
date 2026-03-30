package toolgen

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/matthisholleville/argocd-mcp/internal/openapi"
)

// BuildToolOptions converts an endpoint's parameters and body properties
// into mcp.ToolOption entries for tool registration.
func BuildToolOptions(ep openapi.Endpoint) []mcp.ToolOption {
	var opts []mcp.ToolOption

	// Path and query parameters.
	for _, p := range ep.Parameters {
		switch p.In {
		case "path", "query":
			opts = append(opts, paramToToolOption(p))
		}
	}

	// Flattened body properties.
	for _, bp := range ep.BodyProperties {
		opts = append(opts, bodyPropToToolOption(bp))
	}

	return opts
}

func paramToToolOption(p openapi.Parameter) mcp.ToolOption {
	var propOpts []mcp.PropertyOption
	if p.Description != "" {
		propOpts = append(propOpts, mcp.Description(p.Description))
	}
	if p.Required {
		propOpts = append(propOpts, mcp.Required())
	}

	switch p.Type {
	case "boolean":
		return mcp.WithBoolean(p.Name, propOpts...)
	case "integer", "number":
		return mcp.WithNumber(p.Name, propOpts...)
	default:
		return mcp.WithString(p.Name, propOpts...)
	}
}

func bodyPropToToolOption(bp openapi.BodyProperty) mcp.ToolOption {
	var propOpts []mcp.PropertyOption

	desc := bp.Description
	switch bp.Type {
	case "object":
		if desc != "" {
			desc += " (JSON object)"
		} else {
			desc = "JSON object"
		}
	case "array":
		if desc != "" {
			desc += " (JSON array)"
		} else {
			desc = "JSON array"
		}
	}

	if desc != "" {
		propOpts = append(propOpts, mcp.Description(desc))
	}
	if bp.Required {
		propOpts = append(propOpts, mcp.Required())
	}

	switch bp.Type {
	case "boolean":
		return mcp.WithBoolean(bp.Name, propOpts...)
	case "integer", "number":
		return mcp.WithNumber(bp.Name, propOpts...)
	default:
		// object, array, string all become string params.
		return mcp.WithString(bp.Name, propOpts...)
	}
}
