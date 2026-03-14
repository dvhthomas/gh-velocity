package format

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// --- JSON ---

type jsonStatsOutput struct {
	Repository        string               `json:"repository"`
	Window            JSONWindow           `json:"window"`
	LeadTime          *JSONStats           `json:"lead_time,omitempty"`
	CycleTime         *JSONStats           `json:"cycle_time,omitempty"`
	CycleTimeStrategy string               `json:"cycle_time_strategy,omitempty"`
	Throughput        *jsonThroughput      `json:"throughput,omitempty"`
	Velocity          *jsonVelocitySummary `json:"velocity,omitempty"`
	WIP               *jsonWIP             `json:"wip,omitempty"`
	Quality           *jsonStatsQuality    `json:"quality,omitempty"`
	Warnings          []string             `json:"warnings,omitempty"`
}

type jsonVelocitySummary struct {
	AvgVelocity      float64 `json:"avg_velocity"`
	AvgCompletionPct float64 `json:"avg_completion_pct"`
	StdDev           float64 `json:"std_dev"`
	EffortUnit       string  `json:"effort_unit"`
	IterationCount   int     `json:"iteration_count"`
	CurrentIteration string  `json:"current_iteration,omitempty"`
}

type jsonThroughput struct {
	IssuesClosed int `json:"issues_closed"`
	PRsMerged    int `json:"prs_merged"`
}

type jsonWIP struct {
	Count int `json:"count"`
}

type jsonStatsQuality struct {
	BugCount    int     `json:"bug_count"`
	TotalIssues int     `json:"total_issues"`
	DefectRate  float64 `json:"defect_rate"`
}

// WriteReportJSON writes dashboard metrics as JSON.
func WriteReportJSON(w io.Writer, r model.StatsResult) error {
	out := jsonStatsOutput{
		Repository: r.Repository,
		Window: JSONWindow{
			Since: r.Since.UTC().Format(time.RFC3339),
			Until: r.Until.UTC().Format(time.RFC3339),
		},
	}
	if r.LeadTime != nil {
		s := StatsToJSON(*r.LeadTime)
		out.LeadTime = &s
	}
	if r.CycleTime != nil {
		s := StatsToJSON(*r.CycleTime)
		out.CycleTime = &s
		out.CycleTimeStrategy = r.CycleTimeStrategy
	}
	if r.Throughput != nil {
		out.Throughput = &jsonThroughput{
			IssuesClosed: r.Throughput.IssuesClosed,
			PRsMerged:    r.Throughput.PRsMerged,
		}
	}
	if r.Velocity != nil {
		v := r.Velocity
		n := len(v.History)
		if v.Current != nil {
			n++
		}
		summary := &jsonVelocitySummary{
			AvgVelocity:      v.AvgVelocity,
			AvgCompletionPct: v.AvgCompletion,
			StdDev:           v.StdDev,
			EffortUnit:       v.EffortUnit,
			IterationCount:   n,
		}
		if v.Current != nil {
			summary.CurrentIteration = v.Current.Name
		}
		out.Velocity = summary
	}
	if r.WIPCount != nil {
		out.WIP = &jsonWIP{Count: *r.WIPCount}
	}
	if r.Quality != nil {
		out.Quality = &jsonStatsQuality{
			BugCount:    r.Quality.BugCount,
			TotalIssues: r.Quality.TotalIssues,
			DefectRate:  r.Quality.DefectRate,
		}
	}
	out.Warnings = r.Warnings

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Markdown ---

// WriteReportMarkdown writes dashboard metrics as markdown using an embedded template.
func WriteReportMarkdown(rc RenderContext, r model.StatsResult) error {
	return renderReportMarkdown(rc.Writer, r)
}

// --- Pretty ---

// WriteReportPretty writes dashboard metrics as formatted text.
func WriteReportPretty(rc RenderContext, r model.StatsResult) error {
	w := rc.Writer
	fmt.Fprintf(w, "Report: %s (%s – %s UTC)\n\n",
		r.Repository, r.Since.UTC().Format(time.DateOnly), r.Until.UTC().Format(time.DateOnly))

	if r.LeadTime != nil {
		fmt.Fprintf(w, "  Lead Time:   %s\n", FormatStatsSummary(*r.LeadTime))
	}
	if r.CycleTime != nil {
		fmt.Fprintf(w, "  Cycle Time:  %s\n", FormatStatsSummary(*r.CycleTime))
	} else if r.CycleTimeStrategy != "" {
		switch r.CycleTimeStrategy {
		case model.StrategyIssue:
			fmt.Fprintf(w, "  Cycle Time:  not available (configure lifecycle.in-progress with project_status or match)\n")
		case model.StrategyPR:
			fmt.Fprintf(w, "  Cycle Time:  not available (no closing PRs found)\n")
		}
	}
	if r.Throughput != nil {
		fmt.Fprintf(w, "  Throughput:  %d issues closed, %d PRs merged\n",
			r.Throughput.IssuesClosed, r.Throughput.PRsMerged)
	}
	if r.Velocity != nil {
		fmt.Fprintf(w, "  Velocity:    %s\n", FormatVelocitySummary(*r.Velocity))
	}
	if r.WIPCount != nil {
		fmt.Fprintf(w, "  WIP:         %d items in progress\n", *r.WIPCount)
	}
	if r.Quality != nil {
		fmt.Fprintf(w, "  Quality:     %d bugs / %d issues (%.0f%% defect rate)\n",
			r.Quality.BugCount, r.Quality.TotalIssues, r.Quality.DefectRate*100)
	}

	return nil
}

// FormatVelocitySummary returns a compact velocity summary for the report dashboard.
func FormatVelocitySummary(v model.VelocityResult) string {
	n := len(v.History)
	if v.Current != nil {
		n++
	}
	if n == 0 {
		return "no iterations in window"
	}
	return fmt.Sprintf("%.1f %s/sprint avg, %.0f%% completion (n=%d)",
		v.AvgVelocity, v.EffortUnit, v.AvgCompletion, n)
}

// FormatStatsSummary returns a compact stats summary like "median 3.2d, mean 5.1d, P90 8.1d (n=14, 2 outliers)".
func FormatStatsSummary(s model.Stats) string {
	if s.Count == 0 {
		return "no data"
	}
	result := ""
	if s.Median != nil {
		result += fmt.Sprintf("median %s", FormatDuration(*s.Median))
	}
	if s.Mean != nil {
		if result != "" {
			result += ", "
		}
		result += fmt.Sprintf("mean %s", FormatDuration(*s.Mean))
	}
	if s.P90 != nil {
		if result != "" {
			result += ", "
		}
		result += fmt.Sprintf("P90 %s", FormatDuration(*s.P90))
	}
	suffix := fmt.Sprintf("n=%d", s.Count)
	if s.OutlierCount > 0 {
		suffix += fmt.Sprintf(", %d outliers", s.OutlierCount)
	}
	result += fmt.Sprintf(" (%s)", suffix)
	return result
}
