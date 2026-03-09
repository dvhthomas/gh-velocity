package format

import (
	"fmt"
	"io"
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// WriteReleasePretty writes release metrics as a formatted table to the writer.
func WriteReleasePretty(w io.Writer, rm model.ReleaseMetrics, warnings []string) error {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Release %s\n", rm.Tag))
	b.WriteString(strings.Repeat("=", 60) + "\n\n")

	if rm.PreviousTag != "" {
		b.WriteString(fmt.Sprintf("  Previous:  %s\n", rm.PreviousTag))
		b.WriteString(fmt.Sprintf("  Cadence:   %s\n", FormatDurationPtr(rm.Cadence)))
		if rm.IsHotfix {
			b.WriteString("  ** HOTFIX RELEASE **\n")
		}
		b.WriteString("\n")
	}

	// Composition
	b.WriteString("Composition\n")
	b.WriteString(fmt.Sprintf("  Bug:       %d (%.0f%%)\n", rm.BugCount, rm.BugRatio*100))
	b.WriteString(fmt.Sprintf("  Feature:   %d (%.0f%%)\n", rm.FeatureCount, rm.FeatureRatio*100))
	b.WriteString(fmt.Sprintf("  Other:     %d (%.0f%%)\n", rm.OtherCount, rm.OtherRatio*100))
	b.WriteString(fmt.Sprintf("  Total:     %d\n\n", rm.TotalIssues))

	// Per-issue table
	if len(rm.Issues) > 0 {
		b.WriteString("Issues\n")
		b.WriteString(fmt.Sprintf("  %-6s %-30s %-12s %-12s %-12s %s\n",
			"#", "Title", "Lead Time", "Cycle Time", "Rel. Lag", "Commits"))
		b.WriteString("  " + strings.Repeat("-", 88) + "\n")
		for _, im := range rm.Issues {
			title := im.Issue.Title
			if len(title) > 28 {
				title = title[:28] + ".."
			}
			b.WriteString(fmt.Sprintf("  %-6d %-30s %-12s %-12s %-12s %d\n",
				im.Issue.Number,
				title,
				FormatDurationPtr(im.LeadTime),
				FormatDurationPtr(im.CycleTime),
				FormatDurationPtr(im.ReleaseLag),
				im.CommitCount,
			))
		}
		b.WriteString("\n")
	}

	// Aggregates
	b.WriteString("Aggregates\n")
	b.WriteString(fmt.Sprintf("  %-14s %-12s %-12s %s\n", "Metric", "Mean", "Median", "Std Dev"))
	b.WriteString("  " + strings.Repeat("-", 52) + "\n")
	writePrettyStatsRow(&b, "Lead Time", rm.LeadTimeStats)
	writePrettyStatsRow(&b, "Cycle Time", rm.CycleTimeStats)
	writePrettyStatsRow(&b, "Release Lag", rm.ReleaseLagStats)

	// Warnings
	if len(warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, w := range warnings {
			b.WriteString(fmt.Sprintf("  ! %s\n", w))
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func writePrettyStatsRow(b *strings.Builder, name string, s model.Stats) {
	sd := "--"
	if s.StdDev != nil {
		sd = FormatDuration(*s.StdDev)
	}
	b.WriteString(fmt.Sprintf("  %-14s %-12s %-12s %s\n",
		name,
		FormatDurationPtr(s.Mean),
		FormatDurationPtr(s.Median),
		sd,
	))
}
