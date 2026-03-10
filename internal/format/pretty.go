package format

import (
	"fmt"
	"io"
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
)

// WriteReleasePretty writes release metrics as a formatted table to the writer.
func WriteReleasePretty(w io.Writer, isTTY bool, width int, rm model.ReleaseMetrics, warnings []string) error {
	fmt.Fprintf(w, "Release %s\n", rm.Tag)
	fmt.Fprintln(w, strings.Repeat("=", 60))
	fmt.Fprintln(w)

	if rm.PreviousTag != "" {
		fmt.Fprintf(w, "  Previous:  %s\n", rm.PreviousTag)
		fmt.Fprintf(w, "  Cadence:   %s\n", FormatDurationPtr(rm.Cadence))
		if rm.IsHotfix {
			fmt.Fprintln(w, "  ** HOTFIX RELEASE **")
		}
		fmt.Fprintln(w)
	}

	// Composition
	fmt.Fprintln(w, "Composition")
	fmt.Fprintf(w, "  Bug:       %d (%.0f%%)\n", rm.BugCount, rm.BugRatio*100)
	fmt.Fprintf(w, "  Feature:   %d (%.0f%%)\n", rm.FeatureCount, rm.FeatureRatio*100)
	fmt.Fprintf(w, "  Other:     %d (%.0f%%)\n", rm.OtherCount, rm.OtherRatio*100)
	fmt.Fprintf(w, "  Total:     %d\n\n", rm.TotalIssues)

	// Per-issue table
	if len(rm.Issues) > 0 {
		fmt.Fprintln(w, "Issues")
		tp := NewTable(w, isTTY, width)
		tp.AddHeader([]string{"#", "Title", "Lead Time", "Cycle Time", "Rel. Lag", "Commits", ""})
		for _, im := range rm.Issues {
			flag := ""
			if im.LeadTimeOutlier || im.CycleTimeOutlier {
				flag = "OUTLIER"
			}
			tp.AddField(fmt.Sprintf("%d", im.Issue.Number))
			tp.AddField(im.Issue.Title)
			tp.AddField(FormatDurationPtr(im.LeadTime.Duration))
			tp.AddField(FormatDurationPtr(im.CycleTime.Duration))
			tp.AddField(FormatDurationPtr(im.ReleaseLag.Duration))
			tp.AddField(fmt.Sprintf("%d", im.CommitCount))
			tp.AddField(flag)
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
		}
		fmt.Fprintln(w)
	}

	// Aggregates
	fmt.Fprintln(w, "Aggregates")
	tp := NewTable(w, isTTY, width)
	tp.AddHeader([]string{"Metric", "Mean", "Median", "Std Dev", "P90", "P95", "Outliers"})
	writePrettyStatsRow(tp, "Lead Time", rm.LeadTimeStats)
	writePrettyStatsRow(tp, "Cycle Time", rm.CycleTimeStats)
	writePrettyStatsRow(tp, "Release Lag", rm.ReleaseLagStats)
	if err := tp.Render(); err != nil {
		return err
	}

	// Warnings
	if len(warnings) > 0 {
		fmt.Fprintln(w, "\nWarnings:")
		for _, warn := range warnings {
			fmt.Fprintf(w, "  ! %s\n", warn)
		}
	}

	return nil
}

func writePrettyStatsRow(tp tableprinter.TablePrinter, name string, s model.Stats) {
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
	tp.AddField(name)
	tp.AddField(FormatDurationPtr(s.Mean))
	tp.AddField(FormatDurationPtr(s.Median))
	tp.AddField(sd)
	tp.AddField(p90)
	tp.AddField(p95)
	tp.AddField(outliers)
	tp.EndRow()
}
