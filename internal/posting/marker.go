// Package posting implements idempotent posting of metric output to GitHub.
package posting

import (
	"fmt"
	"strings"
)

const (
	markerPrefix = "<!-- gh-velocity:"
	markerClose  = "<!-- /gh-velocity -->"
)

// MarkerKey returns the marker identifier for a command and context.
// Example: "gh-velocity:lead-time:42"
func MarkerKey(command, context string) string {
	return fmt.Sprintf("gh-velocity:%s:%s", command, context)
}

// WrapWithMarker wraps content with opening and closing HTML comment markers.
// The result constitutes the entire comment/discussion body.
func WrapWithMarker(command, context, content string) string {
	open := fmt.Sprintf("<!-- %s -->", MarkerKey(command, context))
	return open + "\n" + content + "\n" + markerClose + "\n"
}

// FindMarker checks whether the body contains the opening marker for the
// given command and context.
func FindMarker(body, command, context string) bool {
	needle := fmt.Sprintf("<!-- %s -->", MarkerKey(command, context))
	return strings.Contains(body, needle)
}

// InjectMarkedSection replaces an existing marked section in body with
// newSection, or appends newSection at the end if no marker is found.
// The newSection must already be wrapped with markers (via WrapWithMarker).
func InjectMarkedSection(body, command, context, newSection string) string {
	open := fmt.Sprintf("<!-- %s -->", MarkerKey(command, context))
	openIdx := strings.Index(body, open)
	if openIdx == -1 {
		// No existing marker — append with a blank line separator.
		trimmed := strings.TrimRight(body, "\n\r\t ")
		if trimmed == "" {
			return newSection
		}
		return trimmed + "\n\n" + newSection
	}

	// Find the closing marker after the opening one.
	closeIdx := strings.Index(body[openIdx:], markerClose)
	if closeIdx == -1 {
		// Malformed: opening marker without close. Replace from open to end.
		before := strings.TrimRight(body[:openIdx], "\n\r\t ")
		if before == "" {
			return newSection
		}
		return before + "\n\n" + newSection
	}

	// Replace the section between (and including) the markers.
	endIdx := openIdx + closeIdx + len(markerClose)
	// Skip trailing newline after close marker if present.
	if endIdx < len(body) && body[endIdx] == '\n' {
		endIdx++
	}

	before := strings.TrimRight(body[:openIdx], "\n\r\t ")
	after := strings.TrimLeft(body[endIdx:], "\n\r\t ")

	if before == "" && after == "" {
		return newSection
	}
	if before == "" {
		return newSection + "\n\n" + after
	}
	if after == "" {
		return before + "\n\n" + newSection
	}
	return before + "\n\n" + newSection + "\n\n" + after
}
