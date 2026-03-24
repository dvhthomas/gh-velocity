package format

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
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

	// AI-assisted
	if ins.AIAssisted > 0 && len(r.PRsMerged) > 0 {
		pct := ins.AIAssisted * 100 / len(r.PRsMerged)
		lines = append(lines, fmt.Sprintf("%d of %d PRs were AI-assisted (%d%%).", ins.AIAssisted, len(r.PRsMerged), pct))
	}

	// Lead time & cycle time
	if ins.LeadTime != nil {
		lt := fmt.Sprintf("Median lead time: %s (issue created → closed).", formatDuration(*ins.LeadTime))
		if ins.LeadTimeP90 != nil {
			lt = fmt.Sprintf("Median lead time: %s, p90: %s (issue created → closed).", formatDuration(*ins.LeadTime), formatDuration(*ins.LeadTimeP90))
		}
		lines = append(lines, lt)
	}
	if ins.CycleTime != nil {
		lines = append(lines, fmt.Sprintf("Median cycle time: %s (work started → done).", formatDuration(*ins.CycleTime)))
	} else if total > 0 {
		lines = append(lines, "Cycle time not available — run: gh velocity config preflight -R <repo> for setup guidance.")
	}

	// Waiting — prominently surface items blocking progress
	waiting := ins.PRsAwaitingMyReview + ins.PRsNeedingReview + ins.StaleIssues
	if waiting > 0 {
		var parts []string
		if ins.PRsAwaitingMyReview > 0 {
			parts = append(parts, fmt.Sprintf("%d PR(s) awaiting your review", ins.PRsAwaitingMyReview))
		}
		if ins.PRsNeedingReview > 0 {
			parts = append(parts, fmt.Sprintf("%d of your PR(s) waiting for first review", ins.PRsNeedingReview))
		}
		if ins.StaleIssues > 0 {
			parts = append(parts, fmt.Sprintf("%d stale issue(s) (%d+ days idle)", ins.StaleIssues, model.StaleThresholdDays))
		}
		lines = append(lines, fmt.Sprintf("WAITING: %s.", joinParts(parts)))
	}

	// New scope
	if ins.NewIssues > 0 || ins.NewPRs > 0 {
		var parts []string
		if ins.NewIssues > 0 {
			parts = append(parts, fmt.Sprintf("%d issue(s)", ins.NewIssues))
		}
		if ins.NewPRs > 0 {
			parts = append(parts, fmt.Sprintf("%d PR(s)", ins.NewPRs))
		}
		lines = append(lines, fmt.Sprintf("New work picked up: %s.", joinParts(parts)))
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

// joinParts joins 1-3 strings with commas and "and" for the last item.
func joinParts(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
	}
}

// aiSuffix returns a display suffix for AI-assisted PRs.
func aiSuffix(aiAssisted bool) string {
	if aiAssisted {
		return "  [ai]"
	}
	return ""
}

// aiSuffixMarkdown returns a markdown annotation for AI-assisted PRs.
func aiSuffixMarkdown(aiAssisted bool) string {
	if aiAssisted {
		return " `ai`"
	}
	return ""
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
	repoLabel := r.Repo
	if repoLabel == "" {
		repoLabel = "all repositories"
	}
	fmt.Fprintf(w, "My Week — %s (%s)\n", r.Login, repoLabel)
	fmt.Fprintf(w, "  %s to %s\n", r.Since.Format(time.DateOnly), r.Until.Format(time.DateOnly))

	// Insights (prose bullets, not tabular)
	if insights := buildInsightLines(r, ins); len(insights) > 0 {
		fmt.Fprintf(w, "\n── Insights ────────────────────────────────\n\n")
		for _, line := range insights {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}

	// Waiting on: your PRs/issues that are blocked or idle — show early for visibility
	var waitingPRs []model.PR
	for _, pr := range r.PRsNeedingReview {
		waitingPRs = append(waitingPRs, pr)
	}
	var staleIssues []model.Issue
	for _, iss := range r.IssuesOpen {
		if iss.IsStale(r.Until) {
			staleIssues = append(staleIssues, iss)
		}
	}
	if len(waitingPRs) > 0 || len(staleIssues) > 0 {
		fmt.Fprintf(w, "\n── Waiting on ──────────────────────────────\n")
		if len(waitingPRs) > 0 {
			fmt.Fprintf(w, "\nPRs Waiting for Review: %d\n", len(waitingPRs))
			tp := NewTable(w, rc.IsTTY, rc.Width)
			tp.AddHeader([]string{"#", "Title", "Age"})
			for _, pr := range waitingPRs {
				age := model.DaysBetween(pr.CreatedAt, r.Until)
				title := pr.Title + aiSuffix(pr.AIAssisted)
				tp.AddField(FormatItemLink(pr.Number, pr.URL, rc))
				tp.AddField(title)
				tp.AddField(formatAge(age))
				tp.EndRow()
			}
			if err := tp.Render(); err != nil {
				return err
			}
		}
		if len(staleIssues) > 0 {
			fmt.Fprintf(w, "\nStale Issues: %d\n", len(staleIssues))
			tp := NewTable(w, rc.IsTTY, rc.Width)
			tp.AddHeader([]string{"#", "Title", "Idle"})
			for _, iss := range staleIssues {
				staleDays := model.DaysBetween(iss.UpdatedAt, r.Until)
				tp.AddField(FormatItemLink(iss.Number, iss.URL, rc))
				tp.AddField(iss.Title)
				tp.AddField(fmt.Sprintf("%dd", staleDays))
				tp.EndRow()
			}
			if err := tp.Render(); err != nil {
				return err
			}
		}
	}

	// Lookback
	hasLookback := len(r.IssuesClosed) > 0 || len(r.PRsMerged) > 0 || len(r.PRsReviewed) > 0
	fmt.Fprintf(w, "\n── What I shipped ──────────────────────────\n")

	if len(r.IssuesClosed) > 0 {
		fmt.Fprintf(w, "\nIssues Closed: %d\n", len(r.IssuesClosed))
		tp := NewTable(w, rc.IsTTY, rc.Width)
		tp.AddHeader([]string{"#", "Date", "Title"})
		for _, iss := range r.IssuesClosed {
			dateStr := ""
			if iss.ClosedAt != nil {
				dateStr = iss.ClosedAt.Format(time.DateOnly)
			}
			tp.AddField(FormatItemLink(iss.Number, iss.URL, rc))
			tp.AddField(dateStr)
			tp.AddField(iss.Title)
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(w, "\nIssues Closed: 0\n")
		if urls.IssuesClosed != "" {
			fmt.Fprintf(w, "  Verify: %s\n", urls.IssuesClosed)
		}
	}

	if len(r.PRsMerged) > 0 {
		fmt.Fprintf(w, "\nPRs Merged: %d\n", len(r.PRsMerged))
		tp := NewTable(w, rc.IsTTY, rc.Width)
		tp.AddHeader([]string{"#", "Date", "Title"})
		for _, pr := range r.PRsMerged {
			dateStr := ""
			if pr.MergedAt != nil {
				dateStr = pr.MergedAt.Format(time.DateOnly)
			}
			title := pr.Title + aiSuffix(pr.AIAssisted)
			tp.AddField(FormatItemLink(pr.Number, pr.URL, rc))
			tp.AddField(dateStr)
			tp.AddField(title)
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(w, "\nPRs Merged: 0\n")
		if urls.PRsMerged != "" {
			fmt.Fprintf(w, "  Verify: %s\n", urls.PRsMerged)
		}
	}

	if len(r.PRsReviewed) > 0 {
		fmt.Fprintf(w, "\nPRs Reviewed: %d\n", len(r.PRsReviewed))
		tp := NewTable(w, rc.IsTTY, rc.Width)
		tp.AddHeader([]string{"#", "Title"})
		for _, pr := range r.PRsReviewed {
			title := pr.Title + aiSuffix(pr.AIAssisted)
			tp.AddField(FormatItemLink(pr.Number, pr.URL, rc))
			tp.AddField(title)
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(w, "\nPRs Reviewed: 0\n")
		if urls.PRsReviewed != "" {
			fmt.Fprintf(w, "  Verify: %s\n", urls.PRsReviewed)
		}
	}

	if hasLookback {
		if len(r.Releases) > 0 {
			fmt.Fprintf(w, "\nReleases: %d (published in %s)\n", len(r.Releases), r.Repo)
			tp := NewTable(w, rc.IsTTY, rc.Width)
			tp.AddHeader([]string{"Release", "Date"})
			for _, rel := range r.Releases {
				dateStr := rel.CreatedAt.Format(time.DateOnly)
				if rel.PublishedAt != nil {
					dateStr = rel.PublishedAt.Format(time.DateOnly)
				}
				name := rel.Name
				if name == "" {
					name = rel.TagName
				}
				tp.AddField(FormatReleaseLink(name, rel.URL, rc))
				tp.AddField(dateStr)
				tp.EndRow()
			}
			if err := tp.Render(); err != nil {
				return err
			}
		} else if r.Repo == "" {
			fmt.Fprintf(w, "\nReleases: use -R owner/repo to see releases for a specific repo.\n")
		}
	}

	// Lookahead
	hasLookahead := len(r.IssuesOpen) > 0 || len(r.PRsOpen) > 0
	fmt.Fprintf(w, "\n── What's ahead ────────────────────────────\n")

	if len(r.IssuesOpen) > 0 {
		fmt.Fprintf(w, "\nOpen Issues: %d\n", len(r.IssuesOpen))
		tp := NewTable(w, rc.IsTTY, rc.Width)
		tp.AddHeader([]string{"#", "Title", "Status"})
		for _, iss := range r.IssuesOpen {
			s := model.IssueStatus(iss, r.Since, r.Until)
			tp.AddField(FormatItemLink(iss.Number, iss.URL, rc))
			tp.AddField(iss.Title)
			tp.AddField(strings.TrimSpace(formatStatus(s)))
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
		}
	}

	if len(r.PRsOpen) > 0 {
		fmt.Fprintf(w, "\nOpen PRs: %d\n", len(r.PRsOpen))
		tp := NewTable(w, rc.IsTTY, rc.Width)
		tp.AddHeader([]string{"#", "Title", "Status"})
		for _, pr := range r.PRsOpen {
			nr := model.PRNeedsReview(pr, r.PRsNeedingReview)
			s := model.PRStatus(pr, nr, r.Since, r.Until)
			title := pr.Title + aiSuffix(pr.AIAssisted)
			tp.AddField(FormatItemLink(pr.Number, pr.URL, rc))
			tp.AddField(title)
			tp.AddField(strings.TrimSpace(formatStatus(s)))
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
		}
	}

	if !hasLookahead {
		fmt.Fprintf(w, "\n  Nothing planned.\n")
	}

	// Review queue: PRs from others waiting on you
	if len(r.PRsAwaitingMyReview) > 0 {
		fmt.Fprintf(w, "\n── Review queue ────────────────────────────\n")
		fmt.Fprintf(w, "\nAwaiting Your Review: %d\n", len(r.PRsAwaitingMyReview))
		tp := NewTable(w, rc.IsTTY, rc.Width)
		tp.AddHeader([]string{"#", "Title", "Author", "Age"})
		for _, pr := range r.PRsAwaitingMyReview {
			age := model.DaysBetween(pr.CreatedAt, r.Until)
			author := pr.Author
			if author == "" {
				author = "unknown"
			}
			tp.AddField(FormatItemLink(pr.Number, pr.URL, rc))
			tp.AddField(pr.Title)
			tp.AddField(author)
			tp.AddField(formatAge(age))
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
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
	Login     string              `json:"login"`
	Repo      *string             `json:"repo"`
	Since     string              `json:"since"`
	Until     string              `json:"until"`
	Insights  jsonMyWeekInsights  `json:"insights"`
	WaitingOn jsonMyWeekWaitingOn `json:"waiting_on"`
	Lookback  jsonMyWeekLookback  `json:"lookback"`
	Ahead     jsonMyWeekAhead     `json:"ahead"`
	Summary   jsonMyWeekSummary   `json:"summary"`
	Warnings  []string            `json:"warnings,omitempty"`
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

type jsonMyWeekWaitingOn struct {
	PRsNeedingReview []jsonMyWeekWaitingItem `json:"prs_needing_review"`
	StaleIssues      []jsonMyWeekWaitingItem `json:"stale_issues"`
}

type jsonMyWeekWaitingItem struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	AIAssisted bool   `json:"ai_assisted,omitempty"`
	AgeDays    int    `json:"age_days"`
	IdleDays   int    `json:"idle_days,omitempty"` // days since last update (stale issues)
}

type jsonMyWeekReviewItem struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Author  string `json:"author"`
	AgeDays int    `json:"age_days"`
}

type jsonMyWeekItem struct {
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	Date       string   `json:"date,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	AIAssisted bool     `json:"ai_assisted,omitempty"`
}

type jsonMyWeekAheadItem struct {
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	URL        string   `json:"url"`
	Labels     []string `json:"labels,omitempty"`
	AIAssisted bool     `json:"ai_assisted,omitempty"`
	AgeDays    int      `json:"age_days"`
	StaleDays  int      `json:"stale_days,omitempty"`
	Status     string   `json:"status"` // "new", "needs_review", "stale", "active"
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
	LeadTimeP90Hours    *float64 `json:"lead_time_p90_hours,omitempty"`
	CycleTimeHours      *float64 `json:"cycle_time_hours,omitempty"`
	StaleIssues         int      `json:"stale_issues"`
	PRsNeedingReview    int      `json:"prs_needing_review"`
	PRsAwaitingMyReview int      `json:"prs_awaiting_my_review"`
	Releases            int      `json:"releases"`
	NewIssues           int      `json:"new_issues"`
	NewPRs              int      `json:"new_prs"`
	AIAssisted          int      `json:"ai_assisted"`
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
	if ins.LeadTimeP90 != nil {
		h := ins.LeadTimeP90.Hours()
		jsonIns.LeadTimeP90Hours = &h
	}
	if ins.CycleTime != nil {
		h := ins.CycleTime.Hours()
		jsonIns.CycleTimeHours = &h
	}
	jsonIns.AIAssisted = ins.AIAssisted
	var repoPtr *string
	if r.Repo != "" {
		repoPtr = &r.Repo
	}
	out := jsonMyWeekResult{
		Login:    r.Login,
		Repo:     repoPtr,
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
		item := jsonMyWeekItem{Number: pr.Number, Title: pr.Title, URL: pr.URL, Labels: pr.Labels, AIAssisted: pr.AIAssisted}
		if pr.MergedAt != nil {
			item.Date = pr.MergedAt.UTC().Format(time.RFC3339)
		}
		out.Lookback.PRsMerged = append(out.Lookback.PRsMerged, item)
	}
	for _, pr := range r.PRsReviewed {
		out.Lookback.PRsReviewed = append(out.Lookback.PRsReviewed, jsonMyWeekItem{
			Number: pr.Number, Title: pr.Title, URL: pr.URL, Labels: pr.Labels, AIAssisted: pr.AIAssisted,
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
			Number: pr.Number, Title: pr.Title, URL: pr.URL, Labels: pr.Labels, AIAssisted: pr.AIAssisted,
			AgeDays: s.AgeDays, StaleDays: s.StaleDays, Status: s.Status,
		})
	}

	// Waiting on: PRs needing first review, stale issues
	for _, pr := range r.PRsNeedingReview {
		out.WaitingOn.PRsNeedingReview = append(out.WaitingOn.PRsNeedingReview, jsonMyWeekWaitingItem{
			Number:     pr.Number,
			Title:      pr.Title,
			URL:        pr.URL,
			AIAssisted: pr.AIAssisted,
			AgeDays:    model.DaysBetween(pr.CreatedAt, r.Until),
		})
	}
	for _, iss := range r.IssuesOpen {
		if iss.IsStale(r.Until) {
			out.WaitingOn.StaleIssues = append(out.WaitingOn.StaleIssues, jsonMyWeekWaitingItem{
				Number:   iss.Number,
				Title:    iss.Title,
				URL:      iss.URL,
				AgeDays:  model.DaysBetween(iss.CreatedAt, r.Until),
				IdleDays: model.DaysBetween(iss.UpdatedAt, r.Until),
			})
		}
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
