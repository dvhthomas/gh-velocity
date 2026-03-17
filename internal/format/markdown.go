package format

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// maxTitleLength is the maximum length for titles in markdown table cells.
const maxTitleLength = 200

// htmlPolicy strips all HTML tags from content.
var htmlPolicy = bluemonday.StrictPolicy()

// StripMarkdownBold removes **bold** markers from a string for plain-text output.
func StripMarkdownBold(s string) string {
	return strings.ReplaceAll(s, "**", "")
}

// SanitizeMarkdown sanitizes third-party text for safe insertion into markdown tables.
// It strips HTML tags, escapes markdown-breaking characters, removes newlines,
// and truncates to maxTitleLength.
func SanitizeMarkdown(s string) string {
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
