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
	Repository        string            `json:"repository"`
	Window            jsonWindow        `json:"window"`
	LeadTime          *JSONStats        `json:"lead_time,omitempty"`
	CycleTime         *JSONStats        `json:"cycle_time,omitempty"`
	CycleTimeStrategy string            `json:"cycle_time_strategy,omitempty"`
	Throughput        *jsonThroughput   `json:"throughput,omitempty"`
	WIP               *jsonWIP          `json:"wip,omitempty"`
	Quality           *jsonStatsQuality `json:"quality,omitempty"`
	Warnings          []string          `json:"warnings,omitempty"`
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
		Window: jsonWindow{
			Since: r.Since.UTC().Format(time.RFC3339),
			Until: r.Until.UTC().Format(time.RFC3339),
		},
	}
	if r.LeadTime != nil {
		s := statsToJSON(*r.LeadTime)
		out.LeadTime = &s
	}
	if r.CycleTime != nil {
		s := statsToJSON(*r.CycleTime)
		out.CycleTime = &s
		out.CycleTimeStrategy = r.CycleTimeStrategy
	}
	if r.Throughput != nil {
		out.Throughput = &jsonThroughput{
			IssuesClosed: r.Throughput.IssuesClosed,
			PRsMerged:    r.Throughput.PRsMerged,
		}
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

// WriteReportMarkdown writes dashboard metrics as markdown.
func WriteReportMarkdown(w io.Writer, r model.StatsResult) error {
	fmt.Fprintf(w, "## Report: %s (%s – %s UTC)\n\n",
		r.Repository, r.Since.UTC().Format(time.DateOnly), r.Until.UTC().Format(time.DateOnly))

	fmt.Fprintf(w, "| Metric | Value |\n")
	fmt.Fprintf(w, "| --- | --- |\n")

	if r.LeadTime != nil {
		fmt.Fprintf(w, "| Lead Time | %s |\n", formatStatsSummary(*r.LeadTime))
	}
	if r.CycleTime != nil {
		fmt.Fprintf(w, "| Cycle Time | %s |\n", formatStatsSummary(*r.CycleTime))
	}
	if r.Throughput != nil {
		fmt.Fprintf(w, "| Throughput | %d issues closed, %d PRs merged |\n",
			r.Throughput.IssuesClosed, r.Throughput.PRsMerged)
	}
	if r.WIPCount != nil {
		fmt.Fprintf(w, "| WIP | %d items in progress |\n", *r.WIPCount)
	}
	if r.Quality != nil {
		fmt.Fprintf(w, "| Quality | %d bugs / %d issues (%.0f%% defect rate) |\n",
			r.Quality.BugCount, r.Quality.TotalIssues, r.Quality.DefectRate*100)
	}

	if len(r.Warnings) > 0 {
		fmt.Fprintf(w, "\n> [!WARNING]\n")
		for _, msg := range r.Warnings {
			fmt.Fprintf(w, "> %s\n", msg)
		}
	}

	return nil
}

// --- Pretty ---

// WriteReportPretty writes dashboard metrics as formatted text.
func WriteReportPretty(w io.Writer, isTTY bool, width int, r model.StatsResult) error {
	fmt.Fprintf(w, "Report: %s (%s – %s UTC)\n\n",
		r.Repository, r.Since.UTC().Format(time.DateOnly), r.Until.UTC().Format(time.DateOnly))

	if r.LeadTime != nil {
		fmt.Fprintf(w, "  Lead Time:   %s\n", formatStatsSummary(*r.LeadTime))
	}
	if r.CycleTime != nil {
		fmt.Fprintf(w, "  Cycle Time:  %s\n", formatStatsSummary(*r.CycleTime))
	}
	if r.Throughput != nil {
		fmt.Fprintf(w, "  Throughput:  %d issues closed, %d PRs merged\n",
			r.Throughput.IssuesClosed, r.Throughput.PRsMerged)
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

// formatStatsSummary returns a compact stats summary like "median 3.2d, mean 5.1d, P90 8.1d (n=14, 2 outliers)".
func formatStatsSummary(s model.Stats) string {
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
