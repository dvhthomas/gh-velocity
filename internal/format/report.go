package format

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
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
	AvgVelocity      float64       `json:"avg_velocity"`
	AvgCompletionPct float64       `json:"avg_completion_pct"`
	StdDev           float64       `json:"std_dev"`
	EffortUnit       string        `json:"effort_unit"`
	IterationCount   int           `json:"iteration_count"`
	CurrentIteration string        `json:"current_iteration,omitempty"`
	Insights         []JSONInsight `json:"insights,omitempty"`
}

type jsonThroughput struct {
	IssuesClosed int           `json:"issues_closed"`
	PRsMerged    int           `json:"prs_merged"`
	Insights     []JSONInsight `json:"insights,omitempty"`
}

type jsonWIP struct {
	TotalItems     int           `json:"total_items"`
	HumanItemCount int           `json:"human_item_count"`
	BotItemCount   int           `json:"bot_item_count"`
	StaleCount     int           `json:"stale_count"`
	Insights       []JSONInsight `json:"insights,omitempty"`
}

type jsonStatsQuality struct {
	BugCount    int           `json:"bug_count"`
	TotalIssues int           `json:"total_issues"`
	BugRatio  float64       `json:"bug_ratio"`
	Insights    []JSONInsight `json:"insights,omitempty"`
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
		s.Insights = InsightsToJSON(r.LeadTimeInsights)
		out.LeadTime = &s
	}
	if r.CycleTime != nil {
		s := StatsToJSON(*r.CycleTime)
		s.Insights = InsightsToJSON(r.CycleTimeInsights)
		out.CycleTime = &s
		out.CycleTimeStrategy = r.CycleTimeStrategy
	}
	if r.Throughput != nil {
		out.Throughput = &jsonThroughput{
			IssuesClosed: r.Throughput.IssuesClosed,
			PRsMerged:    r.Throughput.PRsMerged,
			Insights:     InsightsToJSON(r.ThroughputInsights),
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
			Insights:         InsightsToJSON(v.Insights),
		}
		if v.Current != nil {
			summary.CurrentIteration = v.Current.Name
		}
		out.Velocity = summary
	}
	if r.WIP != nil {
		out.WIP = &jsonWIP{
			TotalItems:     len(r.WIP.Items),
			HumanItemCount: r.WIP.HumanItemCount,
			BotItemCount:   r.WIP.BotItemCount,
			StaleCount:     r.WIP.Staleness.Stale,
			Insights:       InsightsToJSON(r.WIP.Insights),
		}
	}
	if r.Quality != nil {
		out.Quality = &jsonStatsQuality{
			BugCount:    r.Quality.BugCount,
			TotalIssues: r.Quality.TotalIssues,
			BugRatio:  r.Quality.BugRatio,
			Insights:    InsightsToJSON(r.QualityInsights),
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

// --- HTML ---

//go:embed templates/report-shell.html.tmpl
var reportShellHTML string
var reportShellTmpl = template.Must(template.New("report-shell.html.tmpl").Parse(reportShellHTML))

type reportShellData struct {
	Title   string        // auto-escaped by html/template
	Content template.HTML // pre-rendered, sanitized HTML from goldmark+bluemonday
}

// WriteReportHTML converts markdown content to a self-contained HTML page.
// The markdown is converted to HTML via goldmark, sanitized via bluemonday,
// then wrapped in a styled shell. The title is auto-escaped by html/template.
func WriteReportHTML(w io.Writer, markdownContent string, title string) error {
	htmlBody := MarkdownToHTML(markdownContent)
	return reportShellTmpl.Execute(w, reportShellData{
		Title:   title,
		Content: template.HTML(htmlBody),
	})
}

// --- Pretty ---

// WriteReportPretty writes dashboard metrics as formatted text.
func WriteReportPretty(rc RenderContext, r model.StatsResult) error {
	w := rc.Writer
	fmt.Fprintf(w, "Report: %s (%s – %s UTC)\n\n",
		r.Repository, r.Since.UTC().Format(time.DateOnly), r.Until.UTC().Format(time.DateOnly))

	// Key Findings block.
	groups := buildInsightGroups(r)
	if len(groups) > 0 {
		fmt.Fprintln(w, "Key Findings:")
		for _, group := range groups {
			fmt.Fprintf(w, "\n  %s:\n", group.Section)
			for _, msg := range group.Messages {
				fmt.Fprintf(w, "  → %s\n", msg)
			}
		}
		fmt.Fprintln(w)
	}

	if r.LeadTime != nil {
		fmt.Fprintf(w, "  Lead Time:   %s\n", FormatStatsSummary(*r.LeadTime))
	}
	if r.CycleTime != nil {
		fmt.Fprintf(w, "  Cycle Time:  %s\n", FormatStatsSummary(*r.CycleTime))
	} else if r.CycleTimeStrategy != "" {

		switch r.CycleTimeStrategy {
		case model.StrategyIssue:
			fmt.Fprintf(w, "  Cycle Time:  not available (configure lifecycle.in-progress.match)\n")
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
	if r.WIP != nil {
		stale := r.WIP.Staleness.Stale
		total := len(r.WIP.Items)
		if r.WIP.BotItemCount > 0 {
			if stale > 0 {
				fmt.Fprintf(w, "  WIP:         %d items (%d human, %d bot, %d stale)\n", total, r.WIP.HumanItemCount, r.WIP.BotItemCount, stale)
			} else {
				fmt.Fprintf(w, "  WIP:         %d items (%d human, %d bot)\n", total, r.WIP.HumanItemCount, r.WIP.BotItemCount)
			}
		} else {
			if stale > 0 {
				fmt.Fprintf(w, "  WIP:         %d items (%d stale)\n", total, stale)
			} else {
				fmt.Fprintf(w, "  WIP:         %d items\n", total)
			}
		}
	} else {
		fmt.Fprintf(w, "  WIP:         not configured\n")
	}
	if r.Quality != nil {
		fmt.Fprintf(w, "  Quality:     %d bugs / %d issues (%.0f%% bug ratio)\n",
			r.Quality.BugCount, r.Quality.TotalIssues, r.Quality.BugRatio*100)
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

// buildInsightGroups assembles all insights from StatsResult into grouped sections.
// Velocity insights are accessed via VelocityResult.Insights (different path from other sections).
// Sections with zero insights are omitted.
func buildInsightGroups(r model.StatsResult) []insightGroup {
	type section struct {
		name     string
		insights []model.Insight
	}
	sections := []section{
		{"Lead Time", r.LeadTimeInsights},
		{"Cycle Time", r.CycleTimeInsights},
		{"Throughput", r.ThroughputInsights},
	}
	if r.Velocity != nil {
		sections = append(sections, section{"Velocity", r.Velocity.Insights})
	}
	sections = append(sections, section{"Quality", r.QualityInsights})

	var groups []insightGroup
	for _, s := range sections {
		if len(s.insights) == 0 {
			continue
		}
		msgs := make([]string, len(s.insights))
		for i, ins := range s.insights {
			msgs[i] = ins.Message
		}
		groups = append(groups, insightGroup{Section: s.name, Messages: msgs})
	}
	return groups
}

// FormatStatsSummary returns a compact one-line stats summary for table cells:
// "median 43d 20h, P90 446d 20h, predictability: low (n=21)".
func FormatStatsSummary(s model.Stats) string {
	if s.Count == 0 {
		return "no data"
	}
	result := ""
	if s.Median != nil {
		result += fmt.Sprintf("median %s", FormatDuration(*s.Median))
	}
	if s.P90 != nil {
		if result != "" {
			result += ", "
		}
		result += fmt.Sprintf("P90 %s", FormatDuration(*s.P90))
	}
	if label := PredictabilityLabel(ComputeCV(s)); label != "" {
		if result != "" {
			result += ", "
		}
		result += fmt.Sprintf("predictability: %s", label)
	}
	result += fmt.Sprintf(" (n=%d)", s.Count)
	return result
}

// FormatStatsDetail returns a bullet list of stats for detail sections.
// Each entry is a line like "**Median:** 43d 20h". Suitable for markdown
// (with "- " prefix) or pretty-text (with aligned labels).
func FormatStatsDetail(s model.Stats) []string {
	if s.Count == 0 {
		return []string{"No data"}
	}
	var lines []string
	if s.Median != nil {
		lines = append(lines, fmt.Sprintf("**Median:** %s", FormatDuration(*s.Median)))
	}
	if s.Mean != nil {
		lines = append(lines, fmt.Sprintf("**Mean:** %s", FormatDuration(*s.Mean)))
	}
	if s.P90 != nil {
		lines = append(lines, fmt.Sprintf("**P90:** %s", FormatDuration(*s.P90)))
	}
	cv := ComputeCV(s)
	if label := PredictabilityLabel(cv); label != "" {
		lines = append(lines, fmt.Sprintf("**Predictability:** %s (%s %.1f)", label, DocLink("CV", "/concepts/statistics/#coefficient-of-variation-cv"), *cv))
	}
	sampleSize := fmt.Sprintf("%d", s.Count)
	if s.OutlierCount > 0 {
		sampleSize += fmt.Sprintf(" (%d outliers)", s.OutlierCount)
	}
	lines = append(lines, fmt.Sprintf("**Sample size:** %s", sampleSize))
	return lines
}

// ComputeCV returns the coefficient of variation (stddev/mean) for stats,
// or nil if insufficient data.
func ComputeCV(s model.Stats) *float64 {
	if s.StdDev == nil || s.Mean == nil || *s.Mean == 0 {
		return nil
	}
	cv := float64(*s.StdDev) / float64(*s.Mean)
	cv = math.Round(cv*10) / 10 // one decimal place
	return &cv
}

// PredictabilityLabel returns a human-readable predictability label based on CV.
// Returns "" for high predictability (CV < 0.5) or nil CV.
func PredictabilityLabel(cv *float64) string {
	if cv == nil {
		return ""
	}
	switch {
	case *cv > 1.0:
		return "low"
	case *cv >= 0.5:
		return "moderate"
	default:
		return ""
	}
}
