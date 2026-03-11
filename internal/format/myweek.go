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
	fmt.Fprintf(w, "  %s to %s\n\n", r.Since.Format(time.DateOnly), r.Until.Format(time.DateOnly))

	fmt.Fprintf(w, "Issues Closed: %d\n", len(r.IssuesClosed))
	for _, iss := range r.IssuesClosed {
		fmt.Fprintf(w, "  %s  %s\n", FormatItemLink(iss.Number, iss.URL, rc), iss.Title)
	}

	fmt.Fprintf(w, "\nPRs Merged: %d\n", len(r.PRsMerged))
	for _, pr := range r.PRsMerged {
		fmt.Fprintf(w, "  %s  %s\n", FormatItemLink(pr.Number, pr.URL, rc), pr.Title)
	}

	fmt.Fprintf(w, "\nPRs Reviewed: %d\n", len(r.PRsReviewed))
	for _, pr := range r.PRsReviewed {
		fmt.Fprintf(w, "  %s  %s\n", FormatItemLink(pr.Number, pr.URL, rc), pr.Title)
	}

	if len(r.IssuesClosed) == 0 && len(r.PRsMerged) == 0 && len(r.PRsReviewed) == 0 {
		fmt.Fprintf(w, "\nNo activity in this period.\n")
	}

	return nil
}

// WriteMyWeekMarkdown writes a my-week summary as markdown.
func WriteMyWeekMarkdown(rc RenderContext, r model.MyWeekResult) error {
	w := rc.Writer
	fmt.Fprintf(w, "## My Week — %s\n\n", r.Login)
	fmt.Fprintf(w, "**%s** | %s to %s\n\n", r.Repo, r.Since.Format(time.DateOnly), r.Until.Format(time.DateOnly))

	fmt.Fprintf(w, "### Issues Closed (%d)\n\n", len(r.IssuesClosed))
	if len(r.IssuesClosed) > 0 {
		for _, iss := range r.IssuesClosed {
			fmt.Fprintf(w, "- %s %s\n", FormatItemLink(iss.Number, iss.URL, rc), sanitizeMarkdown(iss.Title))
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	fmt.Fprintf(w, "\n### PRs Merged (%d)\n\n", len(r.PRsMerged))
	if len(r.PRsMerged) > 0 {
		for _, pr := range r.PRsMerged {
			fmt.Fprintf(w, "- %s %s\n", FormatItemLink(pr.Number, pr.URL, rc), sanitizeMarkdown(pr.Title))
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	fmt.Fprintf(w, "\n### PRs Reviewed (%d)\n\n", len(r.PRsReviewed))
	if len(r.PRsReviewed) > 0 {
		for _, pr := range r.PRsReviewed {
			fmt.Fprintf(w, "- %s %s\n", FormatItemLink(pr.Number, pr.URL, rc), sanitizeMarkdown(pr.Title))
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	return nil
}

// jsonMyWeekResult is the JSON serialization of MyWeekResult.
type jsonMyWeekResult struct {
	Login        string          `json:"login"`
	Repo         string          `json:"repo"`
	Since        string          `json:"since"`
	Until        string          `json:"until"`
	IssuesClosed []jsonMyWeekItem `json:"issues_closed"`
	PRsMerged    []jsonMyWeekItem `json:"prs_merged"`
	PRsReviewed  []jsonMyWeekItem `json:"prs_reviewed"`
	Summary      jsonMyWeekSummary `json:"summary"`
}

type jsonMyWeekItem struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	URL    string   `json:"url"`
	Labels []string `json:"labels,omitempty"`
}

type jsonMyWeekSummary struct {
	IssuesClosed int `json:"issues_closed"`
	PRsMerged    int `json:"prs_merged"`
	PRsReviewed  int `json:"prs_reviewed"`
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
		},
	}

	for _, iss := range r.IssuesClosed {
		out.IssuesClosed = append(out.IssuesClosed, jsonMyWeekItem{
			Number: iss.Number, Title: iss.Title, URL: iss.URL, Labels: iss.Labels,
		})
	}
	for _, pr := range r.PRsMerged {
		out.PRsMerged = append(out.PRsMerged, jsonMyWeekItem{
			Number: pr.Number, Title: pr.Title, URL: pr.URL, Labels: pr.Labels,
		})
	}
	for _, pr := range r.PRsReviewed {
		out.PRsReviewed = append(out.PRsReviewed, jsonMyWeekItem{
			Number: pr.Number, Title: pr.Title, URL: pr.URL, Labels: pr.Labels,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
