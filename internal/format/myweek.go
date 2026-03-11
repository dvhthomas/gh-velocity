package format

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// formatAge returns a human-readable age string.
func formatAge(days int) string {
	if days == 0 {
		return "today"
	}
	return fmt.Sprintf("%dd", days)
}

// formatStatus returns a short suffix string for pretty/terminal output.
func formatStatus(s model.ItemStatus) string {
	switch s.Status {
	case model.StatusNew:
		return fmt.Sprintf("  <- new (%s)", formatAge(s.AgeDays))
	case model.StatusNeedsReview:
		return fmt.Sprintf("  <- needs review (%s)", formatAge(s.AgeDays))
	case model.StatusStale:
		return fmt.Sprintf("  <- stale (%s, no update in %dd)", formatAge(s.AgeDays), s.StaleDays)
	default:
		if s.AgeDays > 0 {
			return fmt.Sprintf("  (%dd)", s.AgeDays)
		}
		return ""
	}
}

// formatStatusMarkdown returns a markdown annotation suffix.
func formatStatusMarkdown(s model.ItemStatus) string {
	switch s.Status {
	case model.StatusNew:
		return fmt.Sprintf(" `new %s`", formatAge(s.AgeDays))
	case model.StatusNeedsReview:
		return fmt.Sprintf(" `needs review %s`", formatAge(s.AgeDays))
	case model.StatusStale:
		return fmt.Sprintf(" `stale %s, no update %dd`", formatAge(s.AgeDays), s.StaleDays)
	default:
		if s.AgeDays > 0 {
			return fmt.Sprintf(" *%dd*", s.AgeDays)
		}
		return ""
	}
}

// buildInsightLines returns human-readable observations for 1:1 talking points.
// Returns nil if there's nothing notable to say.
func buildInsightLines(r model.MyWeekResult) []string {
	ins := model.ComputeInsights(r)
	days := model.DaysBetween(r.Since, r.Until)
	var lines []string

	// Shipping velocity
	total := len(r.IssuesClosed) + len(r.PRsMerged)
	if total > 0 {
		lines = append(lines, fmt.Sprintf("Shipped %d items (%d issues closed, %d PRs merged) in %d days.",
			total, len(r.IssuesClosed), len(r.PRsMerged), days))
	}
	if len(r.PRsReviewed) > 0 {
		lines = append(lines, fmt.Sprintf("Reviewed %d PRs.", len(r.PRsReviewed)))
	}

	// Blockers / attention needed
	if ins.PRsAwaitingMyReview > 0 {
		lines = append(lines, fmt.Sprintf("%d PR(s) from others waiting on your review.", ins.PRsAwaitingMyReview))
	}
	if ins.PRsNeedingReview > 0 {
		lines = append(lines, fmt.Sprintf("%d of your open PR(s) waiting for first review.", ins.PRsNeedingReview))
	}
	if ins.StaleIssues > 0 {
		lines = append(lines, fmt.Sprintf("%d open issue(s) stale (no update in %d+ days).", ins.StaleIssues, model.StaleThresholdDays))
	}

	// New scope
	if ins.NewIssues > 0 || ins.NewPRs > 0 {
		parts := []string{}
		if ins.NewIssues > 0 {
			parts = append(parts, fmt.Sprintf("%d issue(s)", ins.NewIssues))
		}
		if ins.NewPRs > 0 {
			parts = append(parts, fmt.Sprintf("%d PR(s)", ins.NewPRs))
		}
		lines = append(lines, fmt.Sprintf("New work picked up: %s.", joinWith(parts, " and ")))
	}

	return lines
}

// joinWith joins strings with sep (like strings.Join but only for 1-2 items).
func joinWith(parts []string, sep string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + sep + parts[1]
}

// WriteMyWeekPretty writes a my-week summary as formatted text.
func WriteMyWeekPretty(rc RenderContext, r model.MyWeekResult) error {
	w := rc.Writer
	fmt.Fprintf(w, "My Week — %s (%s)\n", r.Login, r.Repo)
	fmt.Fprintf(w, "  %s to %s\n", r.Since.Format(time.DateOnly), r.Until.Format(time.DateOnly))

	// Insights
	if insights := buildInsightLines(r); len(insights) > 0 {
		fmt.Fprintf(w, "\n── Insights ────────────────────────────────\n\n")
		for _, line := range insights {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}

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
				s := model.IssueStatus(iss, r.Since, r.Until)
				fmt.Fprintf(w, "  %s  %s%s\n", FormatItemLink(iss.Number, iss.URL, rc), iss.Title, formatStatus(s))
			}
		}

		if len(r.PRsOpen) > 0 {
			fmt.Fprintf(w, "\nOpen PRs: %d\n", len(r.PRsOpen))
			for _, pr := range r.PRsOpen {
				nr := model.PRNeedsReview(pr, r.PRsNeedingReview)
				s := model.PRStatus(pr, nr, r.Since, r.Until)
				fmt.Fprintf(w, "  %s  %s%s\n", FormatItemLink(pr.Number, pr.URL, rc), pr.Title, formatStatus(s))
			}
		}
	}

	// Review queue: PRs from others waiting on you
	if len(r.PRsAwaitingMyReview) > 0 {
		fmt.Fprintf(w, "\n── Review queue ────────────────────────────\n")
		fmt.Fprintf(w, "\nAwaiting Your Review: %d\n", len(r.PRsAwaitingMyReview))
		for _, pr := range r.PRsAwaitingMyReview {
			age := model.DaysBetween(pr.CreatedAt, r.Until)
			author := pr.Author
			if author == "" {
				author = "unknown"
			}
			fmt.Fprintf(w, "  %s  %s  @%s (%s)\n",
				FormatItemLink(pr.Number, pr.URL, rc), pr.Title, author, formatAge(age))
		}
	}

	if !hasLookback && !hasLookahead && len(r.PRsAwaitingMyReview) == 0 {
		fmt.Fprintf(w, "\nNo activity in this period.\n")
	}

	return nil
}

// WriteMyWeekMarkdown writes a my-week summary as markdown.
func WriteMyWeekMarkdown(rc RenderContext, r model.MyWeekResult) error {
	w := rc.Writer
	fmt.Fprintf(w, "## My Week — %s\n\n", r.Login)
	fmt.Fprintf(w, "**%s** | %s to %s\n\n", r.Repo, r.Since.Format(time.DateOnly), r.Until.Format(time.DateOnly))

	// Insights
	if insights := buildInsightLines(r); len(insights) > 0 {
		fmt.Fprintf(w, "### Insights\n\n")
		for _, line := range insights {
			fmt.Fprintf(w, "- %s\n", line)
		}
		fmt.Fprintf(w, "\n")
	}

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
			s := model.IssueStatus(iss, r.Since, r.Until)
			fmt.Fprintf(w, "- %s %s%s\n", FormatItemLink(iss.Number, iss.URL, rc), sanitizeMarkdown(iss.Title), formatStatusMarkdown(s))
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	fmt.Fprintf(w, "\n**Open PRs (%d)**\n\n", len(r.PRsOpen))
	if len(r.PRsOpen) > 0 {
		for _, pr := range r.PRsOpen {
			nr := model.PRNeedsReview(pr, r.PRsNeedingReview)
			s := model.PRStatus(pr, nr, r.Since, r.Until)
			fmt.Fprintf(w, "- %s %s%s\n", FormatItemLink(pr.Number, pr.URL, rc), sanitizeMarkdown(pr.Title), formatStatusMarkdown(s))
		}
	} else {
		fmt.Fprintf(w, "_None_\n")
	}

	// Review queue
	if len(r.PRsAwaitingMyReview) > 0 {
		fmt.Fprintf(w, "\n### Review queue\n\n")
		fmt.Fprintf(w, "**Awaiting Your Review (%d)**\n\n", len(r.PRsAwaitingMyReview))
		for _, pr := range r.PRsAwaitingMyReview {
			age := model.DaysBetween(pr.CreatedAt, r.Until)
			author := pr.Author
			if author == "" {
				author = "unknown"
			}
			fmt.Fprintf(w, "- %s %s — @%s *%s*\n",
				FormatItemLink(pr.Number, pr.URL, rc), sanitizeMarkdown(pr.Title), author, formatAge(age))
		}
	}

	return nil
}

// jsonMyWeekResult is the JSON serialization of MyWeekResult.
type jsonMyWeekResult struct {
	Login    string             `json:"login"`
	Repo     string             `json:"repo"`
	Since    string             `json:"since"`
	Until    string             `json:"until"`
	Insights jsonMyWeekInsights `json:"insights"`
	Lookback jsonMyWeekLookback `json:"lookback"`
	Ahead    jsonMyWeekAhead    `json:"ahead"`
	Summary  jsonMyWeekSummary  `json:"summary"`
}

type jsonMyWeekLookback struct {
	IssuesClosed []jsonMyWeekItem `json:"issues_closed"`
	PRsMerged    []jsonMyWeekItem `json:"prs_merged"`
	PRsReviewed  []jsonMyWeekItem `json:"prs_reviewed"`
}

type jsonMyWeekAhead struct {
	IssuesOpen          []jsonMyWeekAheadItem `json:"issues_open"`
	PRsOpen             []jsonMyWeekAheadItem `json:"prs_open"`
	PRsAwaitingMyReview []jsonMyWeekReviewItem `json:"prs_awaiting_my_review"`
}

type jsonMyWeekReviewItem struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Author  string `json:"author"`
	AgeDays int    `json:"age_days"`
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

type jsonMyWeekInsights struct {
	Lines               []string `json:"lines"`
	StaleIssues         int      `json:"stale_issues"`
	PRsNeedingReview    int      `json:"prs_needing_review"`
	PRsAwaitingMyReview int      `json:"prs_awaiting_my_review"`
	NewIssues           int      `json:"new_issues"`
	NewPRs              int      `json:"new_prs"`
}

// WriteMyWeekJSON writes a my-week summary as JSON.
func WriteMyWeekJSON(w io.Writer, r model.MyWeekResult) error {
	ins := model.ComputeInsights(r)
	out := jsonMyWeekResult{
		Login: r.Login,
		Repo:  r.Repo,
		Since: r.Since.UTC().Format(time.RFC3339),
		Until: r.Until.UTC().Format(time.RFC3339),
		Insights: jsonMyWeekInsights{
			Lines:               buildInsightLines(r),
			StaleIssues:         ins.StaleIssues,
			PRsNeedingReview:    ins.PRsNeedingReview,
			PRsAwaitingMyReview: ins.PRsAwaitingMyReview,
			NewIssues:           ins.NewIssues,
			NewPRs:              ins.NewPRs,
		},
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
		s := model.IssueStatus(iss, r.Since, r.Until)
		out.Ahead.IssuesOpen = append(out.Ahead.IssuesOpen, jsonMyWeekAheadItem{
			Number: iss.Number, Title: iss.Title, URL: iss.URL, Labels: iss.Labels,
			AgeDays: s.AgeDays, StaleDays: s.StaleDays, Status: s.Status,
		})
	}
	for _, pr := range r.PRsOpen {
		nr := model.PRNeedsReview(pr, r.PRsNeedingReview)
		s := model.PRStatus(pr, nr, r.Since, r.Until)
		out.Ahead.PRsOpen = append(out.Ahead.PRsOpen, jsonMyWeekAheadItem{
			Number: pr.Number, Title: pr.Title, URL: pr.URL, Labels: pr.Labels,
			AgeDays: s.AgeDays, StaleDays: s.StaleDays, Status: s.Status,
		})
	}

	// Review queue
	for _, pr := range r.PRsAwaitingMyReview {
		out.Ahead.PRsAwaitingMyReview = append(out.Ahead.PRsAwaitingMyReview, jsonMyWeekReviewItem{
			Number:  pr.Number,
			Title:   pr.Title,
			URL:     pr.URL,
			Author:  pr.Author,
			AgeDays: model.DaysBetween(pr.CreatedAt, r.Until),
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
