package format

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// WriteMyWeekPretty writes a my-week summary as formatted text.
func WriteMyWeekPretty(rc RenderContext, r model.MyWeekResult) error {
	w := rc.Writer
	fmt.Fprintf(w, "My Week — %s (%s)\n", r.Login, r.Repo)
	fmt.Fprintf(w, "  %s to %s\n", r.Since.Format(time.DateOnly), r.Until.Format(time.DateOnly))

	// Lookback
	hasLookback := len(r.IssuesClosed) > 0 || len(r.PRsMerged) > 0 || len(r.PRsReviewed) > 0
	if hasLookback {
		fmt.Fprintf(w, "\n── What I shipped ──────────────────────────\n")

		if len(r.IssuesClosed) > 0 {
			fmt.Fprintf(w, "\nIssues Closed: %d\n", len(r.IssuesClosed))
			for _, iss := range r.IssuesClosed {
				dateStr := ""
				if iss.ClosedAt != nil {
					dateStr = iss.ClosedAt.Format(time.DateOnly) + "  "
				}
				fmt.Fprintf(w, "  %s  %s%s\n", FormatItemLink(iss.Number, iss.URL, rc), dateStr, iss.Title)
			}
		}

		if len(r.PRsMerged) > 0 {
			fmt.Fprintf(w, "\nPRs Merged: %d\n", len(r.PRsMerged))
			for _, pr := range r.PRsMerged {
				dateStr := ""
				if pr.MergedAt != nil {
					dateStr = pr.MergedAt.Format(time.DateOnly) + "  "
				}
				fmt.Fprintf(w, "  %s  %s%s\n", FormatItemLink(pr.Number, pr.URL, rc), dateStr, pr.Title)
			}
		}

		if len(r.PRsReviewed) > 0 {
			fmt.Fprintf(w, "\nPRs Reviewed: %d\n", len(r.PRsReviewed))
			for _, pr := range r.PRsReviewed {
				fmt.Fprintf(w, "  %s  %s\n", FormatItemLink(pr.Number, pr.URL, rc), pr.Title)
			}
		}
	}

	// Lookahead
	hasLookahead := len(r.IssuesOpen) > 0 || len(r.PRsOpen) > 0
	if hasLookahead {
		fmt.Fprintf(w, "\n── What's ahead ────────────────────────────\n")

		if len(r.IssuesOpen) > 0 {
			fmt.Fprintf(w, "\nOpen Issues: %d\n", len(r.IssuesOpen))
			for _, iss := range r.IssuesOpen {
				fmt.Fprintf(w, "  %s  %s\n", FormatItemLink(iss.Number, iss.URL, rc), iss.Title)
			}
		}

		if len(r.PRsOpen) > 0 {
			fmt.Fprintf(w, "\nOpen PRs: %d\n", len(r.PRsOpen))
			for _, pr := range r.PRsOpen {
				fmt.Fprintf(w, "  %s  %s\n", FormatItemLink(pr.Number, pr.URL, rc), pr.Title)
			}
		}
	}

	if !hasLookback && !hasLookahead {
		fmt.Fprintf(w, "\nNo activity in this period.\n")
	}

	return nil
}

// WriteMyWeekMarkdown writes a my-week summary as markdown.
func WriteMyWeekMarkdown(rc RenderContext, r model.MyWeekResult) error {
	w := rc.Writer
	fmt.Fprintf(w, "## My Week — %s\n\n", r.Login)
	fmt.Fprintf(w, "**%s** | %s to %s\n\n", r.Repo, r.Since.Format(time.DateOnly), r.Until.Format(time.DateOnly))

	// Lookback
	fmt.Fprintf(w, "### What I shipped\n\n")

	fmt.Fprintf(w, "**Issues Closed (%d)**\n\n", len(r.IssuesClosed))
	if len(r.IssuesClosed) > 0 {
		for _, iss := range r.IssuesClosed {
			dateStr := ""
			if iss.ClosedAt != nil {
				dateStr = " (" + iss.ClosedAt.Format(time.DateOnly) + ")"
			}
			fmt.Fprintf(w, "- %s %s%s\n", FormatItemLink(iss.Number, iss.URL, rc), sanitizeMarkdown(iss.Title), dateStr)
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	fmt.Fprintf(w, "\n**PRs Merged (%d)**\n\n", len(r.PRsMerged))
	if len(r.PRsMerged) > 0 {
		for _, pr := range r.PRsMerged {
			dateStr := ""
			if pr.MergedAt != nil {
				dateStr = " (" + pr.MergedAt.Format(time.DateOnly) + ")"
			}
			fmt.Fprintf(w, "- %s %s%s\n", FormatItemLink(pr.Number, pr.URL, rc), sanitizeMarkdown(pr.Title), dateStr)
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	fmt.Fprintf(w, "\n**PRs Reviewed (%d)**\n\n", len(r.PRsReviewed))
	if len(r.PRsReviewed) > 0 {
		for _, pr := range r.PRsReviewed {
			fmt.Fprintf(w, "- %s %s\n", FormatItemLink(pr.Number, pr.URL, rc), sanitizeMarkdown(pr.Title))
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	// Lookahead
	fmt.Fprintf(w, "\n### What's ahead\n\n")

	fmt.Fprintf(w, "**Open Issues (%d)**\n\n", len(r.IssuesOpen))
	if len(r.IssuesOpen) > 0 {
		for _, iss := range r.IssuesOpen {
			fmt.Fprintf(w, "- %s %s\n", FormatItemLink(iss.Number, iss.URL, rc), sanitizeMarkdown(iss.Title))
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	fmt.Fprintf(w, "\n**Open PRs (%d)**\n\n", len(r.PRsOpen))
	if len(r.PRsOpen) > 0 {
		for _, pr := range r.PRsOpen {
			fmt.Fprintf(w, "- %s %s\n", FormatItemLink(pr.Number, pr.URL, rc), sanitizeMarkdown(pr.Title))
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	return nil
}

// jsonMyWeekResult is the JSON serialization of MyWeekResult.
type jsonMyWeekResult struct {
	Login    string           `json:"login"`
	Repo     string           `json:"repo"`
	Since    string           `json:"since"`
	Until    string           `json:"until"`
	Lookback jsonMyWeekGroup  `json:"lookback"`
	Ahead    jsonMyWeekGroup  `json:"ahead"`
	Summary  jsonMyWeekSummary `json:"summary"`
}

type jsonMyWeekGroup struct {
	IssuesClosed []jsonMyWeekItem `json:"issues_closed,omitempty"`
	PRsMerged    []jsonMyWeekItem `json:"prs_merged,omitempty"`
	PRsReviewed  []jsonMyWeekItem `json:"prs_reviewed,omitempty"`
	IssuesOpen   []jsonMyWeekItem `json:"issues_open,omitempty"`
	PRsOpen      []jsonMyWeekItem `json:"prs_open,omitempty"`
}

type jsonMyWeekItem struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	URL    string   `json:"url"`
	Date   string   `json:"date,omitempty"` // closed/merged date
	Labels []string `json:"labels,omitempty"`
}

type jsonMyWeekSummary struct {
	IssuesClosed int `json:"issues_closed"`
	PRsMerged    int `json:"prs_merged"`
	PRsReviewed  int `json:"prs_reviewed"`
	IssuesOpen   int `json:"issues_open"`
	PRsOpen      int `json:"prs_open"`
}

// WriteMyWeekJSON writes a my-week summary as JSON.
func WriteMyWeekJSON(w io.Writer, r model.MyWeekResult) error {
	out := jsonMyWeekResult{
		Login: r.Login,
		Repo:  r.Repo,
		Since: r.Since.UTC().Format(time.RFC3339),
		Until: r.Until.UTC().Format(time.RFC3339),
		Summary: jsonMyWeekSummary{
			IssuesClosed: len(r.IssuesClosed),
			PRsMerged:    len(r.PRsMerged),
			PRsReviewed:  len(r.PRsReviewed),
			IssuesOpen:   len(r.IssuesOpen),
			PRsOpen:      len(r.PRsOpen),
		},
	}

	// Lookback
	for _, iss := range r.IssuesClosed {
		item := jsonMyWeekItem{Number: iss.Number, Title: iss.Title, URL: iss.URL, Labels: iss.Labels}
		if iss.ClosedAt != nil {
			item.Date = iss.ClosedAt.UTC().Format(time.RFC3339)
		}
		out.Lookback.IssuesClosed = append(out.Lookback.IssuesClosed, item)
	}
	for _, pr := range r.PRsMerged {
		item := jsonMyWeekItem{Number: pr.Number, Title: pr.Title, URL: pr.URL, Labels: pr.Labels}
		if pr.MergedAt != nil {
			item.Date = pr.MergedAt.UTC().Format(time.RFC3339)
		}
		out.Lookback.PRsMerged = append(out.Lookback.PRsMerged, item)
	}
	for _, pr := range r.PRsReviewed {
		out.Lookback.PRsReviewed = append(out.Lookback.PRsReviewed, jsonMyWeekItem{
			Number: pr.Number, Title: pr.Title, URL: pr.URL, Labels: pr.Labels,
		})
	}

	// Ahead
	for _, iss := range r.IssuesOpen {
		out.Ahead.IssuesOpen = append(out.Ahead.IssuesOpen, jsonMyWeekItem{
			Number: iss.Number, Title: iss.Title, URL: iss.URL, Labels: iss.Labels,
		})
	}
	for _, pr := range r.PRsOpen {
		out.Ahead.PRsOpen = append(out.Ahead.PRsOpen, jsonMyWeekItem{
			Number: pr.Number, Title: pr.Title, URL: pr.URL, Labels: pr.Labels,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
