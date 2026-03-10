// Package format provides output formatters for metrics data.
package format

import (
	"fmt"
	"time"
)

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
