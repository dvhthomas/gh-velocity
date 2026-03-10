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
