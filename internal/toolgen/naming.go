package toolgen

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var camelSplitRe = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// ToToolName converts an OpenAPI operationId to a snake_case MCP tool name.
//
// Examples:
//
//	"ApplicationService_Sync"             → "argocd_application_sync"
//	"ApplicationSetService_List"          → "argocd_application_set_list"
//	"ClusterService_InvalidateCache"      → "argocd_cluster_invalidate_cache"
//	"CustomOp"                            → "argocd_custom_op"
func ToToolName(operationID string) string {
	if operationID == "" {
		return ""
	}

	parts := strings.SplitN(operationID, "_", 2)

	var service, action string
	if len(parts) == 2 {
		service = strings.TrimSuffix(parts[0], "Service")
		action = parts[1]
	} else {
		// No underscore — treat the whole thing as the action.
		action = parts[0]
	}

	name := "argocd"
	if service != "" {
		name += "_" + camelToSnake(service)
	}
	name += "_" + camelToSnake(action)

	return name
}

// camelToSnake converts CamelCase to snake_case.
// "InvalidateCache" → "invalidate_cache"
// "ApplicationSet"  → "application_set"
// "PodLogs2"        → "pod_logs2"
func camelToSnake(s string) string {
	// Insert underscore between lower→upper transitions.
	s = camelSplitRe.ReplaceAllString(s, "${1}_${2}")
	// Handle sequences like "OCIMetadata" → "OCI_Metadata" → "oci_metadata".
	s = splitUpperSequences(s)
	return strings.ToLower(s)
}

// splitUpperSequences handles consecutive uppercase letters.
// "OCIMetadata" → "OCI_Metadata"
// "SSHKnownHosts" → "SSH_Known_Hosts"
func splitUpperSequences(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4)

	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			// Insert underscore if previous is also upper and next is lower.
			// This splits "OCIMetadata" → "OCI_Metadata" but not "OCI" alone.
			if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				b.WriteRune('_')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

// DeduplicateNames takes a slice of names and appends a numeric suffix
// to any duplicates. Empty strings are preserved as-is (not tracked).
// Returns a new slice.
func DeduplicateNames(names []string) []string {
	result := make([]string, len(names))
	seen := make(map[string]int, len(names))

	for i, name := range names {
		if name == "" {
			result[i] = ""
			continue
		}
		if count, exists := seen[name]; exists {
			seen[name] = count + 1
			result[i] = name + "_" + strconv.Itoa(count+1)
		} else {
			seen[name] = 1
			result[i] = name
		}
	}
	return result
}
