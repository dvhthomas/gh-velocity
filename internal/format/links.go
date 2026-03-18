package format

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// FormatItemLink renders an issue/PR reference with a link appropriate to the format.
// Uses OSC 8 hyperlinks for pretty/TTY, markdown links for markdown,
// and plain text for non-TTY or when the URL is empty.
func FormatItemLink(number int, url string, rc RenderContext) string {
	text := fmt.Sprintf("#%d", number)
	switch rc.Format {
	case Markdown:
		if url == "" {
			return text
		}
		return fmt.Sprintf("[%s](%s)", text, stripControlChars(url))
	case JSON:
		return text // URL goes in separate JSON field
	default: // Pretty
		if !rc.IsTTY || url == "" {
			return text
		}
		return ansi.SetHyperlink(stripControlChars(url)) + text + ansi.ResetHyperlink()
	}
}

// FormatReleaseLink renders a release tag with a link appropriate to the format.
func FormatReleaseLink(tag, url string, rc RenderContext) string {
	switch rc.Format {
	case Markdown:
		if url == "" {
			return tag
		}
		return fmt.Sprintf("[%s](%s)", tag, stripControlChars(url))
	case JSON:
		return tag
	default:
		if !rc.IsTTY || url == "" {
			return tag
		}
		return ansi.SetHyperlink(stripControlChars(url)) + tag + ansi.ResetHyperlink()
	}
}

// stripControlChars removes bytes 0x00-0x1f and 0x7f from text.
// Prevents terminal escape sequence injection if untrusted text
// is ever passed to hyperlink rendering.
func stripControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x20 && r != 0x7f {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// DocSiteURL is the base URL for the gh-velocity documentation site.
const DocSiteURL = "https://dvhthomas.github.io/gh-velocity"

// DocLink returns a markdown link to a documentation page.
// anchor is the path after the base URL, e.g., "/concepts/statistics/#...".
func DocLink(text, anchor string) string {
	return fmt.Sprintf("[%s](%s%s)", text, DocSiteURL, anchor)
}

// LinkStatTerms wraps known terms in insight messages with doc links.
// Only call this for markdown output — pretty callers should not use this.
func LinkStatTerms(msg string) string {
	msg = strings.Replace(msg, "(CV ", "("+DocLink("CV", "/concepts/statistics/#coefficient-of-variation-cv")+" ", 1)
	msg = strings.Replace(msg, "(hotfix window)", "("+DocLink("hotfix window", "/reference/metrics/quality/#hotfix-detection")+")", 1)
	msg = strings.Replace(msg, "threshold)", DocLink("threshold", "/reference/metrics/quality/#defect-rate")+")", 1)
	return msg
}

// FormatLabels formats a label slice for display. Returns empty string if no labels.
func FormatLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	return strings.Join(labels, ", ")
}
