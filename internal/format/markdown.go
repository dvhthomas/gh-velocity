package format

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// markdownLinkRe matches [text](url) patterns.
var markdownLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// maxTitleLength is the maximum length for titles in markdown table cells.
const maxTitleLength = 200

// htmlPolicy strips all HTML tags from content (used for markdown sanitization).
var htmlPolicy = bluemonday.StrictPolicy()

// htmlOutputPolicy sanitizes goldmark HTML output: allows safe structural
// elements (tables, details, links, lists, headings, emphasis) but strips
// scripts, event handlers, and dangerous tags. This prevents XSS from
// user-controlled content (issue titles, labels) that flows through the
// markdown-to-HTML pipeline.
var htmlOutputPolicy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowElements("details", "summary")
	p.AllowAttrs("class").OnElements("td", "th", "div", "span")
	return p
}()

// StripMarkdownBold removes **bold** markers from a string for plain-text output.
func StripMarkdownBold(s string) string {
	return strings.ReplaceAll(s, "**", "")
}

// StripMarkdownLinks removes [text](url) links, keeping only the text.
func StripMarkdownLinks(s string) string {
	return markdownLinkRe.ReplaceAllString(s, "$1")
}

// MarkdownLinksToHTML converts [text](url) to <a href="url">text</a>.
func MarkdownLinksToHTML(s string) string {
	return markdownLinkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)
}

// mdRenderer converts GFM markdown (including tables) to HTML.
// WithUnsafe allows raw HTML passthrough (for <details> tags in detail sections).
var mdRenderer = goldmark.New(
	goldmark.WithExtensions(extension.Table),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// MarkdownToHTML converts GitHub-flavored markdown to HTML.
// The output is sanitized via bluemonday to prevent XSS from user-controlled
// content (issue titles, labels) while preserving safe structural HTML
// (tables, links, details, emphasis).
func MarkdownToHTML(md string) string {
	var buf bytes.Buffer
	if err := mdRenderer.Convert([]byte(md), &buf); err != nil {
		return md // fallback to raw text on error
	}
	return htmlOutputPolicy.Sanitize(buf.String())
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
