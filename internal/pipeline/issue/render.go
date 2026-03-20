package issue

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

const cycleTimeDocsURL = "https://dvhthomas.github.io/gh-velocity/guides/cycle-time-setup/"

// WriteMarkdown writes the issue detail as GitHub-flavored markdown.
func WriteMarkdown(rc format.RenderContext, p *Pipeline) error {
	// Facts
	closedFact := ""
	if p.Issue.ClosedAt != nil {
		closedFact = format.FormatTimeFact("closed", *p.Issue.ClosedAt)
	}
	facts := format.FormatFacts(
		format.FormatTimeFact("opened", p.Issue.CreatedAt),
		closedFact,
	)

	// Metrics rows
	ltRow := format.MetricRow{Name: "Lead Time", Status: format.StatusOK, Value: format.FormatMetric(p.LeadTime)}
	if p.LeadTime.Duration == nil {
		ltRow.Status = format.StatusNA
	}
	metrics := []format.MetricRow{ltRow}

	ctRow := format.MetricRow{Name: "Cycle Time"}
	if p.CycleTime.Duration != nil {
		ctRow.Status = format.StatusOK
		ctRow.Value = format.FormatMetric(p.CycleTime)
	} else if p.CycleTime.Start != nil {
		ctRow.Status = format.StatusOK
		ctRow.Value = fmt.Sprintf("in progress since %s", p.CycleTime.Start.Time.UTC().Format(time.DateOnly))
	} else if !p.HasLifecycleMatch {
		ctRow.Status = format.StatusNotConfigured
		ctRow.HelpURL = cycleTimeDocsURL
	} else {
		ctRow.Status = format.StatusNA
	}
	metrics = append(metrics, ctRow)

	for _, lpr := range p.LinkedPRs {
		if lpr.CycleTime.Duration != nil {
			prLink := format.FormatItemLink(lpr.PR.Number, lpr.PR.URL, rc)
			metrics = append(metrics, format.MetricRow{
				Name:  fmt.Sprintf("Eng Cycle Time (%s)", prLink),
				Value: format.FormatMetric(lpr.CycleTime),
			})
		}
	}

	d := format.DetailData{
		Facts:   facts,
		Metrics: metrics,
	}

	return format.WriteDetail(rc.Writer, d)
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

	if p.LeadTime.Duration != nil {
		fmt.Fprintf(w, "  Lead Time:  %s\n", format.FormatMetric(p.LeadTime))
	} else {
		fmt.Fprintf(w, "  Lead Time:  n/a\n")
	}

	ctReason := cycleTimeNAReason(p)
	if p.CycleTime.Duration != nil {
		fmt.Fprintf(w, "  Cycle Time: %s\n", format.FormatMetric(p.CycleTime))
	} else if ctReason != "" {
		fmt.Fprintf(w, "  Cycle Time: %s\n", ctReason)
	} else {
		fmt.Fprintf(w, "  Cycle Time: n/a\n")
	}

	if len(p.LinkedPRs) > 0 {
		fmt.Fprintf(w, "\n  Linked PRs:\n")
		for _, lpr := range p.LinkedPRs {
			ctStr := "n/a"
			if lpr.CycleTime.Duration != nil {
				ctStr = format.FormatMetric(lpr.CycleTime)
			}
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
		return ""
	}
	if p.CycleTimeFiltered {
		return "negative cycle time filtered"
	}
	if !p.HasLifecycleMatch {
		return "not configured"
	}
	return "no in-progress signal found"
}
