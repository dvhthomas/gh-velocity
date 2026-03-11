package format

import (
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/microcosm-cc/bluemonday"
)

// WriteReleaseMarkdown writes release metrics as a markdown table using an embedded template.
func WriteReleaseMarkdown(rc RenderContext, rm model.ReleaseMetrics, warnings []string) error {
	return renderReleaseMarkdown(rc.Writer, rc, rm, warnings)
}

// maxTitleLength is the maximum length for titles in markdown table cells.
const maxTitleLength = 200

// htmlPolicy strips all HTML tags from content.
var htmlPolicy = bluemonday.StrictPolicy()

// sanitizeMarkdown sanitizes third-party text for safe insertion into markdown tables.
// It strips HTML tags, escapes markdown-breaking characters, removes newlines,
// and truncates to maxTitleLength.
func sanitizeMarkdown(s string) string {
	// Strip all HTML tags
	s = htmlPolicy.Sanitize(s)
	// Escape characters that break markdown tables or inject formatting
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	// Truncate to max length
	if len(s) > maxTitleLength {
		s = s[:maxTitleLength-3] + "..."
	}
	return s
}
