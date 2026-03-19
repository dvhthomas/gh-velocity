package issue

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// WriteMarkdown writes the issue detail as GitHub-flavored markdown.
func WriteMarkdown(rc format.RenderContext, p *Pipeline) error {
	w := rc.Writer

	// Header
	fmt.Fprintf(w, "## Issue #%d: %s\n\n", p.IssueNumber, p.Issue.Title)

	// Facts line
	fmt.Fprintf(w, "**Created:** %s UTC", p.Issue.CreatedAt.UTC().Format("2006-01-02 15:04"))
	if p.Issue.ClosedAt != nil {
		fmt.Fprintf(w, " · **Closed:** %s UTC", p.Issue.ClosedAt.UTC().Format("2006-01-02 15:04"))
	}
	fmt.Fprintf(w, " · **Category:** %s\n\n", p.Category)

	// Metrics table
	fmt.Fprintf(w, "| Metric | Value |\n")
	fmt.Fprintf(w, "|--------|-------|\n")

	ltReason := ""
	if p.Issue.ClosedAt == nil {
		ltReason = "issue still open"
	}
	fmt.Fprintf(w, "| Lead Time | %s |\n", formatMetricOrDash(p.LeadTime, ltReason))

	ctReason := cycleTimeNAReason(p)
	fmt.Fprintf(w, "| Cycle Time | %s |\n", formatMetricOrDash(p.CycleTime, ctReason))

	// Linked PRs
	if len(p.LinkedPRs) > 0 {
		fmt.Fprintf(w, "\n### Linked PRs\n\n")
		fmt.Fprintf(w, "| PR | Title | Cycle Time |\n")
		fmt.Fprintf(w, "|----|-------|------------|\n")
		for _, lpr := range p.LinkedPRs {
			prLink := format.FormatItemLink(lpr.PR.Number, lpr.PR.URL, rc)
			ctStr := formatMetricOrDash(lpr.CycleTime, "PR not merged")
			fmt.Fprintf(w, "| %s | %s | %s |\n", prLink, lpr.PR.Title, ctStr)
		}
	}

	return nil
}

// WritePretty writes the issue detail in human-readable text.
func WritePretty(rc format.RenderContext, p *Pipeline) error {
	w := rc.Writer

	fmt.Fprintf(w, "Issue #%d  %s\n", p.IssueNumber, p.Issue.Title)
	fmt.Fprintf(w, "  Created:    %s UTC\n", p.Issue.CreatedAt.UTC().Format(time.RFC3339))
	if p.Issue.ClosedAt != nil {
		fmt.Fprintf(w, "  Closed:     %s UTC\n", p.Issue.ClosedAt.UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(w, "  Category:   %s\n", p.Category)

	ltReason := ""
	if p.Issue.ClosedAt == nil {
		ltReason = "issue still open"
	}
	fmt.Fprintf(w, "  Lead Time:  %s\n", formatMetricOrDash(p.LeadTime, ltReason))

	ctReason := cycleTimeNAReason(p)
	fmt.Fprintf(w, "  Cycle Time: %s\n", formatMetricOrDash(p.CycleTime, ctReason))

	if len(p.LinkedPRs) > 0 {
		fmt.Fprintf(w, "\n  Linked PRs:\n")
		for _, lpr := range p.LinkedPRs {
			ctStr := formatMetricOrDash(lpr.CycleTime, "PR not merged")
			fmt.Fprintf(w, "    PR #%d  %s  (%s)\n", lpr.PR.Number, lpr.PR.Title, ctStr)
		}
	}

	return nil
}

// jsonOutput is the JSON schema for issue detail.
type jsonOutput struct {
	Issue     jsonIssue       `json:"issue"`
	Metrics   jsonMetrics     `json:"metrics"`
	LinkedPRs []jsonLinkedPR  `json:"linked_prs"`
	Warnings  []string        `json:"warnings"`
}

type jsonIssue struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	URL       string     `json:"url"`
	CreatedAt time.Time  `json:"created_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	Category  string     `json:"category"`
}

type jsonMetricValue struct {
	Seconds *float64 `json:"seconds,omitempty"`
	Display string   `json:"display"`
	Signal  string   `json:"signal,omitempty"`
	Status  string   `json:"status,omitempty"`
	Reason  string   `json:"reason,omitempty"`
}

type jsonMetrics struct {
	LeadTime  jsonMetricValue `json:"lead_time"`
	CycleTime jsonMetricValue `json:"cycle_time"`
}

type jsonLinkedPR struct {
	Number    int             `json:"number"`
	Title     string          `json:"title"`
	URL       string          `json:"url"`
	CycleTime jsonMetricValue `json:"cycle_time"`
}

// WriteJSON writes the issue detail as structured JSON.
func WriteJSON(w io.Writer, p *Pipeline) error {
	ltReason := ""
	if p.Issue.ClosedAt == nil {
		ltReason = "issue still open"
	}
	ctReason := cycleTimeNAReason(p)

	out := jsonOutput{
		Issue: jsonIssue{
			Number:    p.IssueNumber,
			Title:     p.Issue.Title,
			URL:       p.Issue.URL,
			CreatedAt: p.Issue.CreatedAt,
			ClosedAt:  p.Issue.ClosedAt,
			Category:  p.Category,
		},
		Metrics: jsonMetrics{
			LeadTime:  metricToJSON(p.LeadTime, ltReason),
			CycleTime: metricToJSON(p.CycleTime, ctReason),
		},
		Warnings: p.Warnings,
	}

	if out.Warnings == nil {
		out.Warnings = []string{}
	}

	for _, lpr := range p.LinkedPRs {
		out.LinkedPRs = append(out.LinkedPRs, jsonLinkedPR{
			Number:    lpr.PR.Number,
			Title:     lpr.PR.Title,
			URL:       lpr.PR.URL,
			CycleTime: metricToJSON(lpr.CycleTime, "PR not merged"),
		})
	}
	if out.LinkedPRs == nil {
		out.LinkedPRs = []jsonLinkedPR{}
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

// cycleTimeNAReason returns the N/A reason string for cycle time.
func cycleTimeNAReason(p *Pipeline) string {
	if p.CycleTime.Duration != nil || p.CycleTime.Start != nil {
		return "" // not N/A
	}
	if p.CycleTimeFiltered {
		return "negative cycle time filtered"
	}
	if p.Strategy == nil {
		return "no cycle time strategy configured"
	}
	return "configure lifecycle.in-progress.match for cycle time"
}
