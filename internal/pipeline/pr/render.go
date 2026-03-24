package pr

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// WriteMarkdown writes the PR detail as GitHub-flavored markdown.
func WriteMarkdown(rc format.RenderContext, p *Pipeline) error {
	// Facts — readable sentence with proper casing and punctuation.
	facts := fmt.Sprintf("Opened by `@%s`%s on %s UTC",
		p.PR.Author, authorTypeSuffix(p.AuthorType),
		p.PR.CreatedAt.UTC().Format("2006-01-02 15:04"))
	if p.PR.MergedAt != nil {
		merger := p.PR.MergedBy
		if merger == "" || merger == p.PR.Author {
			facts += fmt.Sprintf(". Merged %s UTC.", p.PR.MergedAt.UTC().Format("2006-01-02 15:04"))
		} else {
			facts += fmt.Sprintf(". Merged by `@%s` on %s UTC.", merger, p.PR.MergedAt.UTC().Format("2006-01-02 15:04"))
		}
	} else {
		facts += "."
	}

	// Metrics rows
	ctRow := format.MetricRow{Name: "Cycle Time", Status: format.StatusOK, Value: format.FormatMetric(p.CycleTime)}
	if p.CycleTime.Duration == nil {
		ctRow.Status = format.StatusNA
		ctRow.HelpURL = format.DocSiteURL + format.DocPathCycleTimeNA
	}

	ttfrRow := format.MetricRow{Name: "Time to First Review", Status: format.StatusOK}
	if p.ReviewSummary.TimeToFirstReview != nil {
		ttfrRow.Value = format.FormatDuration(*p.ReviewSummary.TimeToFirstReview)
	} else {
		ttfrRow.Status = format.StatusNA
		ttfrRow.HelpURL = format.DocSiteURL + format.DocPathCycleTimeNA
	}

	metrics := []format.MetricRow{
		ctRow,
		ttfrRow,
		{Name: "Review Rounds", Status: format.StatusOK, Value: fmt.Sprintf("%d", p.ReviewSummary.ReviewRounds)},
	}

	// Closed issues as inline links
	var closedIssues []format.DetailLink
	for _, issue := range p.ClosedIssues {
		closedIssues = append(closedIssues, format.DetailLink{
			Text: fmt.Sprintf("#%d", issue.Number),
			URL:  issue.URL,
		})
	}

	d := format.DetailData{
		Facts:        facts,
		Metrics:      metrics,
		ClosedIssues: closedIssues,
	}

	return format.WriteDetail(rc.Writer, d)
}

// WritePretty writes the PR detail in human-readable text.
func WritePretty(rc format.RenderContext, p *Pipeline) error {
	w := rc.Writer

	fmt.Fprintf(w, "PR #%d  %s\n", p.PRNumber, p.PR.Title)
	authorSuffix := authorTypeSuffix(p.AuthorType)
	fmt.Fprintf(w, "  Author:     %s%s\n", p.PR.Author, authorSuffix)
	fmt.Fprintf(w, "  Opened:     %s UTC\n", p.PR.CreatedAt.UTC().Format(time.RFC3339))
	if p.PR.MergedAt != nil {
		fmt.Fprintf(w, "  Merged:     %s UTC\n", p.PR.MergedAt.UTC().Format(time.RFC3339))
	}

	if p.CycleTime.Duration != nil {
		fmt.Fprintf(w, "  Cycle Time: %s\n", format.FormatMetric(p.CycleTime))
	} else {
		fmt.Fprintf(w, "  Cycle Time: n/a\n")
	}

	if p.ReviewSummary.TimeToFirstReview != nil {
		fmt.Fprintf(w, "  First Review: %s\n", format.FormatDuration(*p.ReviewSummary.TimeToFirstReview))
	} else {
		fmt.Fprintf(w, "  First Review: n/a\n")
	}
	fmt.Fprintf(w, "  Review Rounds: %d\n", p.ReviewSummary.ReviewRounds)

	if len(p.ClosedIssues) > 0 {
		fmt.Fprintf(w, "\n  Closed Issues:\n")
		for _, issue := range p.ClosedIssues {
			fmt.Fprintf(w, "    #%d  %s\n", issue.Number, issue.Title)
		}
	}

	return nil
}

// jsonOutput is the JSON schema for PR detail.
type jsonOutput struct {
	PR           jsonPR            `json:"pr"`
	Metrics      jsonMetrics       `json:"metrics"`
	ClosedIssues []jsonClosedIssue `json:"closed_issues"`
	Warnings     []string          `json:"warnings"`
}

type jsonPR struct {
	Number     int        `json:"number"`
	Title      string     `json:"title"`
	URL        string     `json:"url"`
	Author     string     `json:"author"`
	AuthorType string     `json:"author_type"`
	CreatedAt  time.Time  `json:"created_at"`
	MergedAt   *time.Time `json:"merged_at,omitempty"`
}

type jsonMetricValue struct {
	Seconds *float64 `json:"seconds,omitempty"`
	Display string   `json:"display"`
	Signal  string   `json:"signal,omitempty"`
	Status  string   `json:"status,omitempty"`
	Reason  string   `json:"reason,omitempty"`
}

type jsonMetrics struct {
	CycleTime         jsonMetricValue `json:"cycle_time"`
	TimeToFirstReview jsonMetricValue `json:"time_to_first_review"`
	ReviewRounds      int             `json:"review_rounds"`
}

type jsonClosedIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

// WriteJSON writes the PR detail as structured JSON.
func WriteJSON(w io.Writer, p *Pipeline) error {
	ctReason := ""
	if p.PR.MergedAt == nil {
		ctReason = "PR not merged"
	}

	out := jsonOutput{
		PR: jsonPR{
			Number:     p.PRNumber,
			Title:      p.PR.Title,
			URL:        p.PR.URL,
			Author:     p.PR.Author,
			AuthorType: string(p.AuthorType),
			CreatedAt:  p.PR.CreatedAt,
			MergedAt:   p.PR.MergedAt,
		},
		Metrics: jsonMetrics{
			CycleTime:         metricToJSON(p.CycleTime, ctReason),
			TimeToFirstReview: durationToJSON(p.ReviewSummary.TimeToFirstReview, "no reviews"),
			ReviewRounds:      p.ReviewSummary.ReviewRounds,
		},
		Warnings: p.Warnings(),
	}

	if out.Warnings == nil {
		out.Warnings = []string{}
	}

	for _, issue := range p.ClosedIssues {
		out.ClosedIssues = append(out.ClosedIssues, jsonClosedIssue{
			Number: issue.Number,
			Title:  issue.Title,
			URL:    issue.URL,
		})
	}
	if out.ClosedIssues == nil {
		out.ClosedIssues = []jsonClosedIssue{}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// metricToJSON converts a model.Metric to its JSON representation.
func metricToJSON(m model.Metric, naReason string) jsonMetricValue {
	if m.Duration != nil {
		secs := m.Duration.Seconds()
		v := jsonMetricValue{
			Seconds: &secs,
			Display: format.FormatMetric(m),
			Status:  "completed",
		}
		if m.Start != nil {
			v.Signal = format.FormatSignalSummary(m)
		}
		return v
	}
	if m.Start != nil {
		return jsonMetricValue{
			Display: fmt.Sprintf("in progress since %s", m.Start.Time.UTC().Format(time.DateOnly)),
			Status:  "in_progress",
		}
	}
	return jsonMetricValue{
		Display: "—",
		Status:  "not_applicable",
		Reason:  naReason,
	}
}

// durationToJSON converts a *time.Duration to its JSON representation.
func durationToJSON(d *time.Duration, naReason string) jsonMetricValue {
	if d != nil {
		secs := d.Seconds()
		return jsonMetricValue{
			Seconds: &secs,
			Display: format.FormatDuration(*d),
			Status:  "completed",
		}
	}
	return jsonMetricValue{
		Display: "—",
		Status:  "not_applicable",
		Reason:  naReason,
	}
}

// authorTypeSuffix returns a display suffix for non-human author types.
func authorTypeSuffix(at model.AuthorType) string {
	switch at {
	case model.AuthorBot:
		return " (bot)"
	case model.AuthorAgentAssisted:
		return " (agent-assisted)"
	default:
		return ""
	}
}
