package format

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// --- JSON Detail ---

type jsonWIPDetailOutput struct {
	Repository     string              `json:"repository"`
	TotalItems     int                 `json:"total_items"`
	TotalEffort    float64             `json:"total_effort"`
	HumanItemCount int                 `json:"human_item_count"`
	BotItemCount   int                 `json:"bot_item_count"`
	HumanEffort    float64             `json:"human_effort"`
	BotEffort      float64             `json:"bot_effort"`
	StageCounts    []jsonWIPStageCount `json:"stage_counts"`
	Owners         []jsonWIPAssignee   `json:"owners"`
	BotOwners      []jsonWIPAssignee   `json:"bot_owners,omitempty"`
	Staleness      jsonWIPStaleness    `json:"staleness"`
	HumanStaleness jsonWIPStaleness    `json:"human_staleness"`
	BotStaleness   jsonWIPStaleness    `json:"bot_staleness"`
	TeamLimit      *float64            `json:"team_limit,omitempty"`
	PersonLimit    *float64            `json:"person_limit,omitempty"`
	Items          []jsonWIPDetailItem `json:"items"`
	Insights       []JSONInsight       `json:"insights,omitempty"`
	Warnings       []string            `json:"warnings,omitempty"`
}

type jsonWIPStageCount struct {
	Stage         string              `json:"stage"`
	Count         int                 `json:"count"`
	MatcherCounts []jsonMatcherCount  `json:"matcher_counts,omitempty"`
}

type jsonMatcherCount struct {
	Matcher string `json:"matcher"`
	Label   string `json:"label"`
	Count   int    `json:"count"`
}

type jsonWIPAssignee struct {
	Login       string         `json:"login"`
	IsBot       bool           `json:"is_bot,omitempty"`
	ItemCount   int            `json:"item_count"`
	TotalEffort float64        `json:"total_effort"`
	ByStage     map[string]int `json:"by_stage,omitempty"`
	OverLimit   bool           `json:"over_limit,omitempty"`
}

type jsonWIPStaleness struct {
	Active int `json:"active"`
	Aging  int `json:"aging"`
	Stale  int `json:"stale"`
}

type jsonWIPDetailItem struct {
	Number      int      `json:"number,omitempty"`
	Title       string   `json:"title"`
	URL         string   `json:"url,omitempty"`
	Kind        string   `json:"kind"`
	Status      string   `json:"status"`
	Matcher     string   `json:"matched_matcher,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Assignees   []string `json:"assignees,omitempty"`
	EffortValue float64  `json:"effort_value"`
	AgeSeconds  int64    `json:"age_seconds"`
	Age         string   `json:"age"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	Staleness   string   `json:"staleness"`
}

// WriteWIPDetailJSON writes the full WIPResult as JSON.
func WriteWIPDetailJSON(w io.Writer, result model.WIPResult) error {
	out := jsonWIPDetailOutput{
		Repository:     result.Repository,
		TotalItems:     len(result.Items),
		TotalEffort:    result.TotalEffort,
		HumanItemCount: result.HumanItemCount,
		BotItemCount:   result.BotItemCount,
		HumanEffort:    result.HumanEffort,
		BotEffort:      result.BotEffort,
		TeamLimit:      result.TeamLimit,
		PersonLimit:    result.PersonLimit,
		Warnings:       result.Warnings,
		Insights:       InsightsToJSON(result.Insights),
	}

	// Stage counts.
	out.StageCounts = make([]jsonWIPStageCount, 0, len(result.StageCounts))
	for _, sc := range result.StageCounts {
		jsc := jsonWIPStageCount{
			Stage: sc.Stage,
			Count: sc.Count,
		}
		for _, mc := range sc.MatcherCounts {
			jsc.MatcherCounts = append(jsc.MatcherCounts, jsonMatcherCount{
				Matcher: mc.Matcher,
				Label:   mc.Label,
				Count:   mc.Count,
			})
		}
		out.StageCounts = append(out.StageCounts, jsc)
	}

	// Owners (human).
	out.Owners = make([]jsonWIPAssignee, 0, len(result.Assignees))
	for _, a := range result.Assignees {
		out.Owners = append(out.Owners, jsonWIPAssignee{
			Login:       a.Login,
			IsBot:       a.IsBot,
			ItemCount:   a.ItemCount,
			TotalEffort: a.TotalEffort,
			ByStage:     a.ByStage,
			OverLimit:   a.OverLimit,
		})
	}

	// Bot owners.
	if len(result.BotAssignees) > 0 {
		out.BotOwners = make([]jsonWIPAssignee, 0, len(result.BotAssignees))
		for _, a := range result.BotAssignees {
			out.BotOwners = append(out.BotOwners, jsonWIPAssignee{
				Login:       a.Login,
				IsBot:       true,
				ItemCount:   a.ItemCount,
				TotalEffort: a.TotalEffort,
				ByStage:     a.ByStage,
			})
		}
	}

	// Staleness.
	out.Staleness = jsonWIPStaleness{
		Active: result.Staleness.Active,
		Aging:  result.Staleness.Aging,
		Stale:  result.Staleness.Stale,
	}
	out.HumanStaleness = jsonWIPStaleness{
		Active: result.HumanStaleness.Active,
		Aging:  result.HumanStaleness.Aging,
		Stale:  result.HumanStaleness.Stale,
	}
	out.BotStaleness = jsonWIPStaleness{
		Active: result.BotStaleness.Active,
		Aging:  result.BotStaleness.Aging,
		Stale:  result.BotStaleness.Stale,
	}

	// Items.
	out.Items = make([]jsonWIPDetailItem, 0, len(result.Items))
	for _, item := range result.Items {
		updatedAtStr := ""
		if !item.UpdatedAt.IsZero() {
			updatedAtStr = item.UpdatedAt.UTC().Format(time.RFC3339)
		}
		out.Items = append(out.Items, jsonWIPDetailItem{
			Number:      item.Number,
			Title:       item.Title,
			URL:         item.URL,
			Kind:        item.Kind,
			Status:      item.Status,
			Matcher:     item.MatchedMatcher,
			Labels:      item.Labels,
			Assignees:   item.Assignees,
			EffortValue: item.EffortValue,
			AgeSeconds:  int64(item.Age.Seconds()),
			Age:         FormatDuration(item.Age),
			UpdatedAt:   updatedAtStr,
			Staleness:   string(item.Staleness),
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Markdown Detail ---

// WriteWIPDetailMarkdown writes the full WIPResult as markdown.
func WriteWIPDetailMarkdown(rc RenderContext, result model.WIPResult) error {
	w := rc.Writer
	issueCount, prCount := countByKind(result.Items)

	// Summary table — one table with everything at a glance.
	fmt.Fprint(w, "### Summary\n\n")
	hasBots := result.BotItemCount > 0
	if hasBots {
		hIssues, hPRs := countByKindFromSubset(result.Items, false, result)
		bIssues, bPRs := countByKindFromSubset(result.Items, true, result)
		fmt.Fprintf(w, "| | Issues | PRs | Total | %s |\n", DocLink("Stale", "/reference/metrics/staleness/"))
		fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: |")
		fmt.Fprintf(w, "| Human | %d | %d | %d | %d |\n", hIssues, hPRs, result.HumanItemCount, result.HumanStaleness.Stale)
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d |\n",
			DocLink("Bot", "/reference/metrics/staleness/#bot-owners"), bIssues, bPRs, result.BotItemCount, result.BotStaleness.Stale)
		fmt.Fprintf(w, "| **Total** | **%d** | **%d** | **%d** | **%d** |\n",
			issueCount, prCount, len(result.Items), result.Staleness.Stale)
	} else {
		fmt.Fprintln(w, "| | Issues | PRs | Total |")
		fmt.Fprintln(w, "| --- | ---: | ---: | ---: |")
		fmt.Fprintf(w, "| In progress | %d | %d | %d |\n", issueCount, prCount, len(result.Items))
		if result.Staleness.Stale > 0 {
			fmt.Fprintf(w, "| %s | | | %d |\n",
				DocLink("Stale", "/reference/metrics/staleness/"), result.Staleness.Stale)
		}
	}
	fmt.Fprintln(w)

	// Insights — right after summary, before detail tables.
	if len(result.Insights) > 0 {
		fmt.Fprint(w, "### Insights\n\n")
		for _, ins := range result.Insights {
			fmt.Fprintf(w, "- %s\n", LinkStatTerms(ins.Message))
		}
		fmt.Fprintln(w)
	}

	// Stage counts with issue/PR breakdown.
	if len(result.StageCounts) > 0 {
		fmt.Fprint(w, "### Stages\n\n")
		fmt.Fprintln(w, "| Stage | Issues | PRs | Total |")
		fmt.Fprintln(w, "| --- | ---: | ---: | ---: |")
		for _, sc := range result.StageCounts {
			si, sp := countByKindForStage(result.Items, sc.Stage)
			fmt.Fprintf(w, "| **%s** | %d | %d | %d |\n", SanitizeMarkdown(sc.Stage), si, sp, sc.Count)
			for _, mc := range sc.MatcherCounts {
				mi, mp := countByKindForMatcher(result.Items, mc.Matcher)
				fmt.Fprintf(w, "| &nbsp;&nbsp;%s | %d | %d | %d |\n", SanitizeMarkdown(mc.Label), mi, mp, mc.Count)
			}
		}
		fmt.Fprintln(w)
	}

	// Owners (human).
	if len(result.Assignees) > 0 {
		fmt.Fprint(w, "### Owners\n\n")
		fmt.Fprintln(w, "| Owner | Items | Effort |")
		fmt.Fprintln(w, "| --- | ---: | ---: |")
		for _, a := range result.Assignees {
			flag := ""
			if a.OverLimit {
				flag = " :warning:"
			}
			fmt.Fprintf(w, "| %s%s | %d | %.0f |\n", formatOwnerMarkdown(a.Login), flag, a.ItemCount, a.TotalEffort)
		}
		fmt.Fprintln(w)
	}

	// Bot owners.
	if len(result.BotAssignees) > 0 {
		fmt.Fprintf(w, "### %s\n\n", DocLink("Bot Owners", "/reference/metrics/staleness/#bot-owners"))
		fmt.Fprintln(w, "| Owner | Items | Effort |")
		fmt.Fprintln(w, "| --- | ---: | ---: |")
		for _, a := range result.BotAssignees {
			fmt.Fprintf(w, "| %s | %d | %.0f |\n", formatOwnerMarkdown(a.Login), a.ItemCount, a.TotalEffort)
		}
		fmt.Fprintln(w)
	}

	// Staleness breakdown.
	fmt.Fprintf(w, "### %s\n\n", DocLink("Staleness", "/reference/metrics/staleness/"))
	fmt.Fprintln(w, "| Signal | Threshold | Count |")
	fmt.Fprintln(w, "| --- | --- | ---: |")
	fmt.Fprintf(w, "| Active | updated < 3 days ago | %d |\n", result.Staleness.Active)
	fmt.Fprintf(w, "| Aging | updated 3–7 days ago | %d |\n", result.Staleness.Aging)
	fmt.Fprintf(w, "| Stale | updated > 7 days ago | %d |\n", result.Staleness.Stale)
	fmt.Fprintln(w)

	// WIP limits — apply to human effort.
	if result.TeamLimit != nil || result.PersonLimit != nil {
		fmt.Fprint(w, "### WIP Limits\n\n")
		if result.TeamLimit != nil {
			status := "within limit"
			if result.HumanEffort > *result.TeamLimit {
				status = fmt.Sprintf("**exceeded** (%.0f/%.0f)", result.HumanEffort, *result.TeamLimit)
			} else {
				status = fmt.Sprintf("%.0f/%.0f", result.HumanEffort, *result.TeamLimit)
			}
			fmt.Fprintf(w, "- Team limit (human): %s\n", status)
		}
		if result.PersonLimit != nil {
			fmt.Fprintf(w, "- Person limit: %.0f\n", *result.PersonLimit)
		}
		fmt.Fprintln(w)
	}

	// Per-item table — sorted by needs-attention, capped.
	needsAttention := sortWIPByNeedsAttention(result.Items)
	total := len(needsAttention)
	capped := total > maxWIPDetailItems
	if capped {
		needsAttention = needsAttention[:maxWIPDetailItems]
	}

	if len(needsAttention) > 0 {
		if capped {
			fmt.Fprintf(w, "### Items (showing %d of %d — oldest stale first)\n\n", maxWIPDetailItems, total)
		} else {
			fmt.Fprint(w, "### Items\n\n")
		}
		fmt.Fprintln(w, "| Signal | # | Title | Kind | Status | Age | Last Activity |")
		fmt.Fprintln(w, "| --- | ---: | --- | --- | --- | --- | --- |")
		for _, item := range needsAttention {
			link := ""
			if item.Number > 0 {
				link = FormatItemLink(item.Number, item.URL, rc)
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s |\n",
				string(item.Staleness),
				link,
				SanitizeMarkdown(item.Title),
				item.Kind,
				item.Status,
				FormatDuration(item.Age),
				formatLastActivity(item.UpdatedAt),
			)
		}
		if capped {
			fmt.Fprintf(w, "\n*%d more items not shown. Use `--format json` for complete data.*\n", total-maxWIPDetailItems)
		}
		fmt.Fprintln(w)
	}

	return nil
}

// --- Pretty Detail ---

// WriteWIPDetailPretty writes the full WIPResult as formatted text.
func WriteWIPDetailPretty(rc RenderContext, result model.WIPResult) error {
	w := rc.Writer
	issueCount, prCount := countByKind(result.Items)

	termWidth := rc.Width
	if termWidth == 0 {
		termWidth = 80
	}

	// Shared style funcs.
	numericRightStyle := func(row, col int) lipgloss.Style {
		s := lipgloss.NewStyle().Padding(0, 1)
		if col >= 1 {
			s = s.Align(lipgloss.Right)
		}
		if row == table.HeaderRow {
			s = s.Bold(true)
		}
		return s
	}

	// Summary table.
	fmt.Fprintf(w, "Work in Progress: %s\n", result.Repository)
	hasBots := result.BotItemCount > 0
	if hasBots {
		hIssues, hPRs := countByKindFromSubset(result.Items, false, result)
		bIssues, bPRs := countByKindFromSubset(result.Items, true, result)
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			Headers("", "Issues", "PRs", "Total", "Stale").
			Width(termWidth).
			StyleFunc(numericRightStyle).
			Row("Human", itoa(hIssues), itoa(hPRs), itoa(result.HumanItemCount), itoa(result.HumanStaleness.Stale)).
			Row("Bot", itoa(bIssues), itoa(bPRs), itoa(result.BotItemCount), itoa(result.BotStaleness.Stale)).
			Row("Total", itoa(issueCount), itoa(prCount), itoa(len(result.Items)), itoa(result.Staleness.Stale))
		fmt.Fprintln(w, t)
	} else {
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			Headers("", "Issues", "PRs", "Total").
			Width(termWidth).
			StyleFunc(numericRightStyle).
			Row("In progress", itoa(issueCount), itoa(prCount), itoa(len(result.Items)))
		if result.Staleness.Stale > 0 {
			t.Row("Stale", "", "", itoa(result.Staleness.Stale))
		}
		fmt.Fprintln(w, t)
	}
	fmt.Fprintln(w)

	if len(result.Items) == 0 {
		fmt.Fprintln(w, "  No items in progress.")
		return nil
	}

	// Insights — right after summary.
	if len(result.Insights) > 0 {
		fmt.Fprintln(w, "Insights:")
		for _, ins := range result.Insights {
			fmt.Fprintf(w, "  → %s\n", ins.Message)
		}
		fmt.Fprintln(w)
	}

	// Stage counts table with issue/PR breakdown.
	if len(result.StageCounts) > 0 {
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			Headers("Stage", "Issues", "PRs", "Total").
			Width(termWidth).
			StyleFunc(numericRightStyle)
		for _, sc := range result.StageCounts {
			si, sp := countByKindForStage(result.Items, sc.Stage)
			t.Row(sc.Stage, itoa(si), itoa(sp), itoa(sc.Count))
			for _, mc := range sc.MatcherCounts {
				mi, mp := countByKindForMatcher(result.Items, mc.Matcher)
				t.Row("  "+mc.Label, itoa(mi), itoa(mp), itoa(mc.Count))
			}
		}
		fmt.Fprintln(w, t)
		fmt.Fprintln(w)
	}

	// Owners (human).
	if len(result.Assignees) > 0 {
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			Headers("Owner", "Items", "Effort", "").
			Width(termWidth).
			StyleFunc(numericRightStyle)
		for _, a := range result.Assignees {
			flag := ""
			if a.OverLimit {
				flag = "⚠ over limit"
			}
			t.Row(a.Login, itoa(a.ItemCount), ftoa(a.TotalEffort), flag)
		}
		fmt.Fprintln(w, t)
		fmt.Fprintln(w)
	}

	// Bot owners.
	if len(result.BotAssignees) > 0 {
		fmt.Fprintln(w, "Bot Owners:")
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			Headers("Owner", "Items", "Effort").
			Width(termWidth).
			StyleFunc(numericRightStyle)
		for _, a := range result.BotAssignees {
			t.Row(a.Login, itoa(a.ItemCount), ftoa(a.TotalEffort))
		}
		fmt.Fprintln(w, t)
		fmt.Fprintln(w)
	}

	// Staleness table.
	{
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			Headers("Signal", "Threshold", "Count").
			Width(termWidth).
			StyleFunc(func(row, col int) lipgloss.Style {
				s := lipgloss.NewStyle().Padding(0, 1)
				if col == 2 {
					s = s.Align(lipgloss.Right)
				}
				if row == table.HeaderRow {
					s = s.Bold(true)
				}
				return s
			}).
			Row("Active", "updated < 3 days ago", itoa(result.Staleness.Active)).
			Row("Aging", "updated 3–7 days ago", itoa(result.Staleness.Aging)).
			Row("Stale", "updated > 7 days ago", itoa(result.Staleness.Stale))
		fmt.Fprintln(w, t)
	}

	// WIP limits — apply to human effort.
	if result.TeamLimit != nil {
		if result.HumanEffort > *result.TeamLimit {
			fmt.Fprintf(w, "Human WIP:  %.0f / %.0f (EXCEEDED)\n", result.HumanEffort, *result.TeamLimit)
		} else {
			fmt.Fprintf(w, "Human WIP:  %.0f / %.0f\n", result.HumanEffort, *result.TeamLimit)
		}
	}
	if result.PersonLimit != nil {
		fmt.Fprintf(w, "Person limit: %.0f\n", *result.PersonLimit)
	}
	fmt.Fprintln(w)

	// Per-item table — sorted by needs-attention, capped.
	needsAttention := sortWIPByNeedsAttention(result.Items)
	total := len(needsAttention)
	capped := total > maxWIPDetailItems
	if capped {
		needsAttention = needsAttention[:maxWIPDetailItems]
	}

	if len(needsAttention) > 0 {
		if capped {
			fmt.Fprintf(w, "Items (%d of %d — oldest stale first):\n", maxWIPDetailItems, total)
		} else {
			fmt.Fprintln(w, "Items:")
		}
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			Headers("Signal", "#", "Title", "Kind", "Status", "Age", "Last Activity").
			Width(termWidth).
			StyleFunc(func(row, col int) lipgloss.Style {
				s := lipgloss.NewStyle().Padding(0, 1)
				if col == 1 { // right-align # column
					s = s.Align(lipgloss.Right)
				}
				if row == table.HeaderRow {
					s = s.Bold(true)
				}
				return s
			})
		for _, item := range needsAttention {
			num := ""
			if item.Number > 0 {
				num = FormatItemLink(item.Number, item.URL, rc)
			}
			t.Row(
				string(item.Staleness),
				num,
				item.Title,
				item.Kind,
				item.Status,
				FormatDuration(item.Age),
				formatLastActivity(item.UpdatedAt),
			)
		}
		fmt.Fprintln(w, t)
		if capped {
			fmt.Fprintf(w, "  %d more items not shown. Use --format json for complete data.\n", total-maxWIPDetailItems)
		}
	}
	return nil
}

// formatOwnerMarkdown renders an owner handle for markdown output without
// triggering a live GitHub @mention notification.
// "unassigned" is returned as-is (not a GitHub handle).
func formatOwnerMarkdown(login string) string {
	if login == "unassigned" {
		return login
	}
	login = strings.TrimPrefix(login, "@")
	return "`@" + login + "`"
}

// countByKind counts issues and PRs in the item list.
func countByKind(items []model.WIPItem) (issues, prs int) {
	for _, item := range items {
		if item.Kind == "PR" {
			prs++
		} else {
			issues++
		}
	}
	return
}

// countByKindFromSubset counts issues/PRs for either human or bot items.
// isBot=true counts bot items; isBot=false counts human items.
func countByKindFromSubset(items []model.WIPItem, isBot bool, result model.WIPResult) (issues, prs int) {
	// Build a set of bot logins from BotAssignees.
	botLogins := make(map[string]bool)
	for _, a := range result.BotAssignees {
		botLogins[strings.ToLower(a.Login)] = true
	}
	for _, item := range items {
		itemIsBot := false
		if len(item.Assignees) > 0 {
			allBot := true
			for _, a := range item.Assignees {
				if !botLogins[strings.ToLower(a)] {
					allBot = false
					break
				}
			}
			itemIsBot = allBot
		}
		if itemIsBot != isBot {
			continue
		}
		if item.Kind == "PR" {
			prs++
		} else {
			issues++
		}
	}
	return
}

// countByKindForStage counts issues/PRs matching a specific stage.
func countByKindForStage(items []model.WIPItem, stage string) (issues, prs int) {
	for _, item := range items {
		if item.Status != stage {
			continue
		}
		if item.Kind == "PR" {
			prs++
		} else {
			issues++
		}
	}
	return
}

// countByKindForMatcher counts issues/PRs matching a specific matcher.
func countByKindForMatcher(items []model.WIPItem, matcher string) (issues, prs int) {
	for _, item := range items {
		if item.MatchedMatcher != matcher {
			continue
		}
		if item.Kind == "PR" {
			prs++
		} else {
			issues++
		}
	}
	return
}

func itoa(n int) string  { return fmt.Sprintf("%d", n) }
func ftoa(f float64) string { return fmt.Sprintf("%.0f", f) }
