package format

import (
	"fmt"
	"io"
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/microcosm-cc/bluemonday"
)

// WriteReleaseMarkdown writes release metrics as a markdown table.
func WriteReleaseMarkdown(w io.Writer, rm model.ReleaseMetrics, warnings []string) error {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Release %s\n\n", rm.Tag))

	if rm.PreviousTag != "" {
		b.WriteString(fmt.Sprintf("**Previous:** %s | **Cadence:** %s",
			rm.PreviousTag, FormatDurationPtr(rm.Cadence)))
		if rm.IsHotfix {
			b.WriteString(" | **Hotfix**")
		}
		b.WriteString("\n\n")
	}

	// Composition
	b.WriteString("### Composition\n\n")
	b.WriteString("| Type | Count | Ratio |\n")
	b.WriteString("| --- | ---: | ---: |\n")
	b.WriteString(fmt.Sprintf("| Bug | %d | %.0f%% |\n", rm.BugCount, rm.BugRatio*100))
	b.WriteString(fmt.Sprintf("| Feature | %d | %.0f%% |\n", rm.FeatureCount, rm.FeatureRatio*100))
	b.WriteString(fmt.Sprintf("| Other | %d | %.0f%% |\n", rm.OtherCount, rm.OtherRatio*100))
	b.WriteString(fmt.Sprintf("| **Total** | **%d** | |\n\n", rm.TotalIssues))

	// Per-issue table
	if len(rm.Issues) > 0 {
		b.WriteString("### Issues\n\n")
		b.WriteString("| # | Title | Lead Time | Cycle Time | Release Lag | Commits | |\n")
		b.WriteString("| ---: | --- | --- | --- | --- | ---: | --- |\n")
		for _, im := range rm.Issues {
			title := sanitizeMarkdown(im.Issue.Title)
			flag := ""
			if im.LeadTimeOutlier || im.CycleTimeOutlier {
				flag = "OUTLIER"
			}
			b.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %d | %s |\n",
				im.Issue.Number,
				title,
				FormatDurationPtr(im.LeadTime.Duration),
				FormatDurationPtr(im.CycleTime.Duration),
				FormatDurationPtr(im.ReleaseLag.Duration),
				im.CommitCount,
				flag,
			))
		}
		b.WriteString("\n")
	}

	// Aggregates
	b.WriteString("### Aggregates\n\n")
	b.WriteString("| Metric | Mean | Median | Std Dev | P90 | P95 | Outliers |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | ---: |\n")
	writeStatsRow(&b, "Lead Time", rm.LeadTimeStats)
	writeStatsRow(&b, "Cycle Time", rm.CycleTimeStats)
	writeStatsRow(&b, "Release Lag", rm.ReleaseLagStats)

	// Warnings
	if len(warnings) > 0 {
		b.WriteString("\n### Warnings\n\n")
		for _, w := range warnings {
			b.WriteString(fmt.Sprintf("- %s\n", w))
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func writeStatsRow(b *strings.Builder, name string, s model.Stats) {
	sd := "--"
	if s.StdDev != nil {
		sd = FormatDuration(*s.StdDev)
	}
	p90 := "--"
	if s.P90 != nil {
		p90 = FormatDuration(*s.P90)
	}
	p95 := "--"
	if s.P95 != nil {
		p95 = FormatDuration(*s.P95)
	}
	outliers := "--"
	if s.OutlierCutoff != nil {
		outliers = fmt.Sprintf("%d", s.OutlierCount)
	}
	b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
		name,
		FormatDurationPtr(s.Mean),
		FormatDurationPtr(s.Median),
		sd,
		p90,
		p95,
		outliers,
	))
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
