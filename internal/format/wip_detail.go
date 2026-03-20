package format

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// --- JSON Detail ---

type jsonWIPDetailOutput struct {
	Repository  string              `json:"repository"`
	TotalItems  int                 `json:"total_items"`
	TotalEffort float64             `json:"total_effort"`
	StageCounts []jsonWIPStageCount `json:"stage_counts"`
	Assignees   []jsonWIPAssignee   `json:"assignees"`
	Staleness   jsonWIPStaleness    `json:"staleness"`
	TeamLimit   *float64            `json:"team_limit,omitempty"`
	PersonLimit *float64            `json:"person_limit,omitempty"`
	Items       []jsonWIPDetailItem `json:"items"`
	Insights    []JSONInsight       `json:"insights,omitempty"`
	Warnings    []string            `json:"warnings,omitempty"`
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
		Repository:  result.Repository,
		TotalItems:  len(result.Items),
		TotalEffort: result.TotalEffort,
		TeamLimit:   result.TeamLimit,
		PersonLimit: result.PersonLimit,
		Warnings:    result.Warnings,
		Insights:    InsightsToJSON(result.Insights),
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

	// Assignees.
	out.Assignees = make([]jsonWIPAssignee, 0, len(result.Assignees))
	for _, a := range result.Assignees {
		out.Assignees = append(out.Assignees, jsonWIPAssignee{
			Login:       a.Login,
			ItemCount:   a.ItemCount,
			TotalEffort: a.TotalEffort,
			ByStage:     a.ByStage,
			OverLimit:   a.OverLimit,
		})
	}

	// Staleness.
	out.Staleness = jsonWIPStaleness{
		Active: result.Staleness.Active,
		Aging:  result.Staleness.Aging,
		Stale:  result.Staleness.Stale,
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
	sorted := sortWIPByAgeDesc(result.Items)

	staleCount := result.Staleness.Stale
	if staleCount > 0 {
		fmt.Fprintf(w, "**%d items** in progress (%d stale)\n\n", len(result.Items), staleCount)
	} else {
		fmt.Fprintf(w, "**%d items** in progress\n\n", len(result.Items))
	}

	// Stage counts.
	if len(result.StageCounts) > 0 {
		fmt.Fprint(w, "### Stages\n\n")
		fmt.Fprintln(w, "| Stage | Count |")
		fmt.Fprintln(w, "| --- | ---: |")
		for _, sc := range result.StageCounts {
			fmt.Fprintf(w, "| %s | %d |\n", SanitizeMarkdown(sc.Stage), sc.Count)
			for _, mc := range sc.MatcherCounts {
				fmt.Fprintf(w, "| &nbsp;&nbsp;%s | %d |\n", SanitizeMarkdown(mc.Label), mc.Count)
			}
		}
		fmt.Fprintln(w)
	}

	// Assignees.
	if len(result.Assignees) > 0 {
		fmt.Fprint(w, "### Assignees\n\n")
		fmt.Fprintln(w, "| Assignee | Items | Effort |")
		fmt.Fprintln(w, "| --- | ---: | ---: |")
		for _, a := range result.Assignees {
			flag := ""
			if a.OverLimit {
				flag = " :warning:"
			}
			fmt.Fprintf(w, "| %s%s | %d | %.0f |\n", SanitizeMarkdown(a.Login), flag, a.ItemCount, a.TotalEffort)
		}
		fmt.Fprintln(w)
	}

	// Staleness.
	fmt.Fprint(w, "### Staleness\n\n")
	fmt.Fprintln(w, "| Signal | Count |")
	fmt.Fprintln(w, "| --- | ---: |")
	fmt.Fprintf(w, "| Active (<3d) | %d |\n", result.Staleness.Active)
	fmt.Fprintf(w, "| Aging (3-7d) | %d |\n", result.Staleness.Aging)
	fmt.Fprintf(w, "| Stale (>7d) | %d |\n", result.Staleness.Stale)
	fmt.Fprintln(w)

	// WIP limits.
	if result.TeamLimit != nil || result.PersonLimit != nil {
		fmt.Fprint(w, "### WIP Limits\n\n")
		if result.TeamLimit != nil {
			status := "within limit"
			if result.TotalEffort > *result.TeamLimit {
				status = fmt.Sprintf("**exceeded** (%.0f/%.0f)", result.TotalEffort, *result.TeamLimit)
			} else {
				status = fmt.Sprintf("%.0f/%.0f", result.TotalEffort, *result.TeamLimit)
			}
			fmt.Fprintf(w, "- Team limit: %s\n", status)
		}
		if result.PersonLimit != nil {
			fmt.Fprintf(w, "- Person limit: %.0f\n", *result.PersonLimit)
		}
		fmt.Fprintln(w)
	}

	// Insights.
	if len(result.Insights) > 0 {
		fmt.Fprint(w, "### Insights\n\n")
		for _, ins := range result.Insights {
			fmt.Fprintf(w, "- %s\n", ins.Message)
		}
		fmt.Fprintln(w)
	}

	// Per-item table.
	if len(sorted) > 0 {
		fmt.Fprint(w, "### Items\n\n")
		fmt.Fprintln(w, "| # | Title | Kind | Status | Age | Last Activity | Signal |")
		fmt.Fprintln(w, "| ---: | --- | --- | --- | --- | --- | --- |")
		for _, item := range sorted {
			link := ""
			if item.Number > 0 {
				link = FormatItemLink(item.Number, item.URL, rc)
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s |\n",
				link,
				SanitizeMarkdown(item.Title),
				item.Kind,
				item.Status,
				FormatDuration(item.Age),
				formatLastActivity(item.UpdatedAt),
				string(item.Staleness),
			)
		}
		fmt.Fprintln(w)
	}

	return nil
}

// --- Pretty Detail ---

// WriteWIPDetailPretty writes the full WIPResult as formatted text.
func WriteWIPDetailPretty(rc RenderContext, result model.WIPResult) error {
	w := rc.Writer
	sorted := sortWIPByAgeDesc(result.Items)

	// Summary line.
	staleCount := result.Staleness.Stale
	if staleCount > 0 {
		fmt.Fprintf(w, "Work in Progress: %s (%d items, %d stale)\n\n", result.Repository, len(result.Items), staleCount)
	} else {
		fmt.Fprintf(w, "Work in Progress: %s (%d items)\n\n", result.Repository, len(result.Items))
	}

	if len(result.Items) == 0 {
		fmt.Fprintln(w, "  No items in progress.")
		return nil
	}

	// Stage counts.
	if len(result.StageCounts) > 0 {
		fmt.Fprintln(w, "Stages:")
		for _, sc := range result.StageCounts {
			fmt.Fprintf(w, "  %s: %d\n", sc.Stage, sc.Count)
			for _, mc := range sc.MatcherCounts {
				fmt.Fprintf(w, "    %s: %d\n", mc.Label, mc.Count)
			}
		}
		fmt.Fprintln(w)
	}

	// Assignees.
	if len(result.Assignees) > 0 {
		fmt.Fprintln(w, "Assignees:")
		tp := NewTable(w, rc.IsTTY, rc.Width)
		tp.AddHeader([]string{"Assignee", "Items", "Effort", ""})
		for _, a := range result.Assignees {
			flag := ""
			if a.OverLimit {
				flag = "(over limit)"
			}
			tp.AddField(a.Login)
			tp.AddField(fmt.Sprintf("%d", a.ItemCount))
			tp.AddField(fmt.Sprintf("%.0f", a.TotalEffort))
			tp.AddField(flag)
			tp.EndRow()
		}
		if err := tp.Render(); err != nil {
			return err
		}
		fmt.Fprintln(w)
	}

	// Staleness.
	fmt.Fprintf(w, "Staleness:  active=%d  aging=%d  stale=%d\n",
		result.Staleness.Active, result.Staleness.Aging, result.Staleness.Stale)

	// WIP limits.
	if result.TeamLimit != nil {
		if result.TotalEffort > *result.TeamLimit {
			fmt.Fprintf(w, "Team WIP:   %.0f / %.0f (EXCEEDED)\n", result.TotalEffort, *result.TeamLimit)
		} else {
			fmt.Fprintf(w, "Team WIP:   %.0f / %.0f\n", result.TotalEffort, *result.TeamLimit)
		}
	}
	if result.PersonLimit != nil {
		fmt.Fprintf(w, "Person limit: %.0f\n", *result.PersonLimit)
	}
	fmt.Fprintln(w)

	// Insights.
	if len(result.Insights) > 0 {
		fmt.Fprintln(w, "Insights:")
		for _, ins := range result.Insights {
			fmt.Fprintf(w, "  -> %s\n", ins.Message)
		}
		fmt.Fprintln(w)
	}

	// Per-item table.
	tp := NewTable(w, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"#", "Title", "Kind", "Status", "Age", "Last Activity", "Signal"})
	for _, item := range sorted {
		num := ""
		if item.Number > 0 {
			num = FormatItemLink(item.Number, item.URL, rc)
		}
		tp.AddField(num)
		tp.AddField(item.Title)
		tp.AddField(item.Kind)
		tp.AddField(item.Status)
		tp.AddField(FormatDuration(item.Age))
		tp.AddField(formatLastActivity(item.UpdatedAt))
		tp.AddField(string(item.Staleness))
		tp.EndRow()
	}
	return tp.Render()
}
