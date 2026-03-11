package format

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// annotation describes why an open item deserves attention.
type annotation struct {
	Kind    string // "new", "needs_review", "stale", or ""
	AgeDays int    // days since creation
	StaleDays int  // days since last update (0 if not stale)
}

const (
	staleThresholdDays = 7 // no update in 7+ days = stale
)

// annotateIssue computes an annotation for an open issue.
func annotateIssue(iss model.Issue, since, now time.Time) annotation {
	a := annotation{AgeDays: daysBetween(iss.CreatedAt, now)}
	switch {
	case !iss.CreatedAt.Before(since): // created within --since window
		a.Kind = "new"
	case daysBetween(iss.UpdatedAt, now) >= staleThresholdDays:
		a.Kind = "stale"
		a.StaleDays = daysBetween(iss.UpdatedAt, now)
	}
	return a
}

// annotatePR computes an annotation for an open PR.
func annotatePR(pr model.PR, needsReview bool, since, now time.Time) annotation {
	a := annotation{AgeDays: daysBetween(pr.CreatedAt, now)}
	switch {
	case !pr.CreatedAt.Before(since):
		a.Kind = "new"
	case needsReview && a.AgeDays >= 2: // give 1 day grace before flagging
		a.Kind = "needs_review"
	}
	// PRs don't have UpdatedAt, so we don't flag stale — use needs_review instead.
	return a
}

func daysBetween(a, b time.Time) int {
	return int(math.Floor(b.Sub(a).Hours() / 24))
}

// prNeedsReview returns true if the PR number appears in the needsReview set.
func prNeedsReview(pr model.PR, needsReview []model.PR) bool {
	for _, nr := range needsReview {
		if nr.Number == pr.Number {
			return true
		}
	}
	return false
}

// formatAnnotation returns a short suffix string for pretty/terminal output.
func formatAnnotation(a annotation) string {
	switch a.Kind {
	case "new":
		return fmt.Sprintf("  <- new (%dd ago)", a.AgeDays)
	case "needs_review":
		return fmt.Sprintf("  <- needs review (%dd)", a.AgeDays)
	case "stale":
		return fmt.Sprintf("  <- stale (%dd, no update in %dd)", a.AgeDays, a.StaleDays)
	default:
		if a.AgeDays > 0 {
			return fmt.Sprintf("  (%dd)", a.AgeDays)
		}
		return ""
	}
}

// formatAnnotationMarkdown returns a markdown annotation suffix.
func formatAnnotationMarkdown(a annotation) string {
	switch a.Kind {
	case "new":
		return fmt.Sprintf(" `new %dd ago`", a.AgeDays)
	case "needs_review":
		return fmt.Sprintf(" `needs review %dd`", a.AgeDays)
	case "stale":
		return fmt.Sprintf(" `stale %dd, no update %dd`", a.AgeDays, a.StaleDays)
	default:
		if a.AgeDays > 0 {
			return fmt.Sprintf(" *%dd*", a.AgeDays)
		}
		return ""
	}
}

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
				a := annotateIssue(iss, r.Since, r.Until)
				fmt.Fprintf(w, "  %s  %s%s\n", FormatItemLink(iss.Number, iss.URL, rc), iss.Title, formatAnnotation(a))
			}
		}

		if len(r.PRsOpen) > 0 {
			fmt.Fprintf(w, "\nOpen PRs: %d\n", len(r.PRsOpen))
			for _, pr := range r.PRsOpen {
				nr := prNeedsReview(pr, r.PRsNeedingReview)
				a := annotatePR(pr, nr, r.Since, r.Until)
				fmt.Fprintf(w, "  %s  %s%s\n", FormatItemLink(pr.Number, pr.URL, rc), pr.Title, formatAnnotation(a))
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
			a := annotateIssue(iss, r.Since, r.Until)
			fmt.Fprintf(w, "- %s %s%s\n", FormatItemLink(iss.Number, iss.URL, rc), sanitizeMarkdown(iss.Title), formatAnnotationMarkdown(a))
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	fmt.Fprintf(w, "\n**Open PRs (%d)**\n\n", len(r.PRsOpen))
	if len(r.PRsOpen) > 0 {
		for _, pr := range r.PRsOpen {
			nr := prNeedsReview(pr, r.PRsNeedingReview)
			a := annotatePR(pr, nr, r.Since, r.Until)
			fmt.Fprintf(w, "- %s %s%s\n", FormatItemLink(pr.Number, pr.URL, rc), sanitizeMarkdown(pr.Title), formatAnnotationMarkdown(a))
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	return nil
}

// jsonMyWeekResult is the JSON serialization of MyWeekResult.
type jsonMyWeekResult struct {
	Login    string            `json:"login"`
	Repo     string            `json:"repo"`
	Since    string            `json:"since"`
	Until    string            `json:"until"`
	Lookback jsonMyWeekLookback `json:"lookback"`
	Ahead    jsonMyWeekAhead   `json:"ahead"`
	Summary  jsonMyWeekSummary `json:"summary"`
}

type jsonMyWeekLookback struct {
	IssuesClosed []jsonMyWeekItem `json:"issues_closed"`
	PRsMerged    []jsonMyWeekItem `json:"prs_merged"`
	PRsReviewed  []jsonMyWeekItem `json:"prs_reviewed"`
}

type jsonMyWeekAhead struct {
	IssuesOpen []jsonMyWeekAheadItem `json:"issues_open"`
	PRsOpen    []jsonMyWeekAheadItem `json:"prs_open"`
}

type jsonMyWeekItem struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	URL    string   `json:"url"`
	Date   string   `json:"date,omitempty"`
	Labels []string `json:"labels,omitempty"`
}

type jsonMyWeekAheadItem struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Labels    []string `json:"labels,omitempty"`
	AgeDays   int      `json:"age_days"`
	StaleDays int      `json:"stale_days,omitempty"`
	Status    string   `json:"status"` // "new", "needs_review", "stale", "active"
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

	// Ahead — with annotations
	for _, iss := range r.IssuesOpen {
		a := annotateIssue(iss, r.Since, r.Until)
		status := a.Kind
		if status == "" {
			status = "active"
		}
		out.Ahead.IssuesOpen = append(out.Ahead.IssuesOpen, jsonMyWeekAheadItem{
			Number: iss.Number, Title: iss.Title, URL: iss.URL, Labels: iss.Labels,
			AgeDays: a.AgeDays, StaleDays: a.StaleDays, Status: status,
		})
	}
	for _, pr := range r.PRsOpen {
		nr := prNeedsReview(pr, r.PRsNeedingReview)
		a := annotatePR(pr, nr, r.Since, r.Until)
		status := a.Kind
		if status == "" {
			status = "active"
		}
		out.Ahead.PRsOpen = append(out.Ahead.PRsOpen, jsonMyWeekAheadItem{
			Number: pr.Number, Title: pr.Title, URL: pr.URL, Labels: pr.Labels,
			AgeDays: a.AgeDays, StaleDays: a.StaleDays, Status: status,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
