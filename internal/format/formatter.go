// Package format provides output formatters for metrics data.
package format

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// RenderContext consolidates common formatter parameters to prevent
// parameter list explosion as output concerns (links, labels) grow.
type RenderContext struct {
	Writer io.Writer
	Format Format
	IsTTY  bool
	Width  int
	Owner  string // repository owner, for constructing URLs
	Repo   string // repository name
}

// Format represents an output format type.
type Format string

const (
	JSON     Format = "json"
	Pretty   Format = "pretty"
	Markdown Format = "markdown"
)

// ParseFormat validates and returns a Format from a string.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case JSON, Pretty, Markdown:
		return Format(s), nil
	default:
		return "", fmt.Errorf("invalid format %q: must be json, pretty, or markdown", s)
	}
}

// FormatDuration formats a duration for human-readable output.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		return "-" + FormatDuration(-d)
	}
	if d == 0 {
		return "0s"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// FormatDurationPtr formats an optional duration, returning "N/A" for nil.
func FormatDurationPtr(d *time.Duration) string {
	if d == nil {
		return "N/A"
	}
	return FormatDuration(*d)
}

// FormatCycleStatus formats a cycle time duration. Returns "in progress"
// when started is true but duration is nil, "N/A" when not started.
func FormatCycleStatus(d *time.Duration, started bool) string {
	if d != nil {
		return FormatDuration(*d)
	}
	if started {
		return "in progress"
	}
	return "N/A"
}

// FormatMetric formats a Metric's duration with its signal summary.
// Example: "10d 13h  (created -> closed)"
func FormatMetric(m model.Metric) string {
	dur := FormatMetricDuration(m)
	summary := FormatSignalSummary(m)
	if summary != "" {
		return dur + "  " + summary
	}
	return dur
}

// FormatMetricDuration formats just the duration portion of a Metric.
func FormatMetricDuration(m model.Metric) string {
	if m.Duration != nil {
		return FormatDuration(*m.Duration)
	}
	if m.Start != nil {
		return "in progress"
	}
	return "N/A"
}

// FormatSignalSummary returns a parenthesized signal summary like "(created -> closed)".
func FormatSignalSummary(m model.Metric) string {
	if m.Start == nil {
		return ""
	}
	startLabel := shortSignal(m.Start.Signal)
	if m.End == nil {
		return "(" + startLabel + " -> ...)"
	}
	endLabel := shortSignal(m.End.Signal)
	return "(" + startLabel + " -> " + endLabel + ")"
}

// FormatStringSlice formats a string slice as a YAML-style inline array.
func FormatStringSlice(ss []string) string {
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// shortSignal returns a short display name for a signal constant.
func shortSignal(signal string) string {
	// Strip the common prefixes for brevity
	signal = strings.TrimPrefix(signal, "issue-")
	signal = strings.TrimPrefix(signal, "pr-")
	signal = strings.TrimPrefix(signal, "release-")
	return signal
}
