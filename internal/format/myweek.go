package format

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
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
func buildInsightLines(r model.MyWeekResult, ins model.MyWeekInsights) []string {
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
	if ins.Releases > 0 {
		lines = append(lines, fmt.Sprintf("%d release(s) published.", ins.Releases))
	}

	// Lead time & cycle time
	if ins.LeadTime != nil {
		lines = append(lines, fmt.Sprintf("Median lead time: %s (issue created → closed).", formatDuration(*ins.LeadTime)))
	}
	if ins.CycleTime != nil {
		lines = append(lines, fmt.Sprintf("Median cycle time: %s (work started → done).", formatDuration(*ins.CycleTime)))
	} else if total > 0 {
		lines = append(lines, "Cycle time not available — run: gh velocity config preflight -R <repo> for setup guidance.")
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

// formatDuration renders a duration as a human-friendly string (e.g., "3d 4h", "12h").
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	if days > 0 && hours > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return "<1h"
}

// joinWith joins strings with sep (like strings.Join but only for 1-2 items).
func joinWith(parts []string, sep string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + sep + parts[1]
}

// MyWeekSearchURLs holds verify URLs for each lookback section.
type MyWeekSearchURLs struct {
	IssuesClosed string
	PRsMerged    string
	PRsReviewed  string
}

// WriteMyWeekPretty writes a my-week summary as formatted text.
func WriteMyWeekPretty(rc RenderContext, r model.MyWeekResult, ins model.MyWeekInsights, urls MyWeekSearchURLs) error {
	w := rc.Writer
	fmt.Fprintf(w, "My Week — %s (%s)\n", r.Login, r.Repo)
	fmt.Fprintf(w, "  %s to %s\n", r.Since.Format(time.DateOnly), r.Until.Format(time.DateOnly))

	// Insights
	if insights := buildInsightLines(r, ins); len(insights) > 0 {
		fmt.Fprintf(w, "\n── Insights ────────────────────────────────\n\n")
		for _, line := range insights {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}

	// Lookback
	hasLookback := len(r.IssuesClosed) > 0 || len(r.PRsMerged) > 0 || len(r.PRsReviewed) > 0
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
	} else {
		fmt.Fprintf(w, "\nIssues Closed: 0\n")
		if urls.IssuesClosed != "" {
			fmt.Fprintf(w, "  Verify: %s\n", urls.IssuesClosed)
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
	} else {
		fmt.Fprintf(w, "\nPRs Merged: 0\n")
		if urls.PRsMerged != "" {
			fmt.Fprintf(w, "  Verify: %s\n", urls.PRsMerged)
		}
	}

	if len(r.PRsReviewed) > 0 {
		fmt.Fprintf(w, "\nPRs Reviewed: %d\n", len(r.PRsReviewed))
		for _, pr := range r.PRsReviewed {
			fmt.Fprintf(w, "  %s  %s\n", FormatItemLink(pr.Number, pr.URL, rc), pr.Title)
		}
	} else {
		fmt.Fprintf(w, "\nPRs Reviewed: 0\n")
		if urls.PRsReviewed != "" {
			fmt.Fprintf(w, "  Verify: %s\n", urls.PRsReviewed)
		}
	}

	if hasLookback {

		if len(r.Releases) > 0 {
			fmt.Fprintf(w, "\nReleases: %d\n", len(r.Releases))
			for _, rel := range r.Releases {
				dateStr := rel.CreatedAt.Format(time.DateOnly)
				if rel.PublishedAt != nil {
					dateStr = rel.PublishedAt.Format(time.DateOnly)
				}
				name := rel.Name
				if name == "" {
					name = rel.TagName
				}
				fmt.Fprintf(w, "  %s  %s\n", dateStr, name)
			}
		}
	}

	// Lookahead
	hasLookahead := len(r.IssuesOpen) > 0 || len(r.PRsOpen) > 0
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

	if !hasLookahead {
		fmt.Fprintf(w, "\n  Nothing planned.\n")
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

// WriteMyWeekMarkdown writes a my-week summary as markdown using an embedded template.
func WriteMyWeekMarkdown(rc RenderContext, r model.MyWeekResult, ins model.MyWeekInsights, urls MyWeekSearchURLs) error {
	return renderMyWeekMarkdown(rc.Writer, rc, r, ins, urls)
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
	Warnings []string           `json:"warnings,omitempty"`
}

type jsonMyWeekLookback struct {
	IssuesClosed []jsonMyWeekItem     `json:"issues_closed"`
	PRsMerged    []jsonMyWeekItem     `json:"prs_merged"`
	Releases     []jsonMyWeekRelease  `json:"releases,omitempty"`
	PRsReviewed  []jsonMyWeekItem     `json:"prs_reviewed"`
	SearchURLs   jsonMyWeekSearchURLs `json:"search_urls"`
}

type jsonMyWeekSearchURLs struct {
	IssuesClosed string `json:"issues_closed"`
	PRsMerged    string `json:"prs_merged"`
	PRsReviewed  string `json:"prs_reviewed"`
}

type jsonMyWeekAhead struct {
	IssuesOpen          []jsonMyWeekAheadItem  `json:"issues_open"`
	PRsOpen             []jsonMyWeekAheadItem  `json:"prs_open"`
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
	LeadTimeHours       *float64 `json:"lead_time_hours,omitempty"`
	CycleTimeHours      *float64 `json:"cycle_time_hours,omitempty"`
	StaleIssues         int      `json:"stale_issues"`
	PRsNeedingReview    int      `json:"prs_needing_review"`
	PRsAwaitingMyReview int      `json:"prs_awaiting_my_review"`
	Releases            int      `json:"releases"`
	NewIssues           int      `json:"new_issues"`
	NewPRs              int      `json:"new_prs"`
}

type jsonMyWeekRelease struct {
	Tag          string `json:"tag"`
	Name         string `json:"name"`
	URL          string `json:"url,omitempty"`
	PublishedAt  string `json:"published_at"`
	IsPrerelease bool   `json:"is_prerelease,omitempty"`
}

// WriteMyWeekJSON writes a my-week summary as JSON.
func WriteMyWeekJSON(w io.Writer, r model.MyWeekResult, ins model.MyWeekInsights, urls MyWeekSearchURLs, warnings []string) error {
	jsonIns := jsonMyWeekInsights{
		Lines:               buildInsightLines(r, ins),
		StaleIssues:         ins.StaleIssues,
		PRsNeedingReview:    ins.PRsNeedingReview,
		PRsAwaitingMyReview: ins.PRsAwaitingMyReview,
		Releases:            ins.Releases,
		NewIssues:           ins.NewIssues,
		NewPRs:              ins.NewPRs,
	}
	if ins.LeadTime != nil {
		h := ins.LeadTime.Hours()
		jsonIns.LeadTimeHours = &h
	}
	if ins.CycleTime != nil {
		h := ins.CycleTime.Hours()
		jsonIns.CycleTimeHours = &h
	}
	out := jsonMyWeekResult{
		Login:    r.Login,
		Repo:     r.Repo,
		Since:    r.Since.UTC().Format(time.RFC3339),
		Until:    r.Until.UTC().Format(time.RFC3339),
		Insights: jsonIns,
		Summary: jsonMyWeekSummary{
			IssuesClosed: len(r.IssuesClosed),
			PRsMerged:    len(r.PRsMerged),
			PRsReviewed:  len(r.PRsReviewed),
			IssuesOpen:   len(r.IssuesOpen),
			PRsOpen:      len(r.PRsOpen),
		},
		Warnings: warnings,
	}

	// Lookback search URLs
	out.Lookback.SearchURLs = jsonMyWeekSearchURLs(urls)

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
	for _, rel := range r.Releases {
		pubAt := rel.CreatedAt.UTC().Format(time.RFC3339)
		if rel.PublishedAt != nil {
			pubAt = rel.PublishedAt.UTC().Format(time.RFC3339)
		}
		name := rel.Name
		if name == "" {
			name = rel.TagName
		}
		out.Lookback.Releases = append(out.Lookback.Releases, jsonMyWeekRelease{
			Tag: rel.TagName, Name: name, URL: rel.URL,
			PublishedAt: pubAt, IsPrerelease: rel.IsPrerelease,
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
