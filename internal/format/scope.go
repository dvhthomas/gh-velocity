package format

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// WriteScopePretty writes scope results as a formatted table to the writer.
func WriteScopePretty(w io.Writer, result *model.ScopeResult) error {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Scope: %s", result.Tag))
	if result.PreviousTag != "" {
		b.WriteString(fmt.Sprintf(" (since %s)", result.PreviousTag))
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Per-strategy breakdown
	for _, sr := range result.Strategies {
		b.WriteString(fmt.Sprintf("Strategy: %s (%d items)\n", sr.Name, len(sr.Items)))
		if len(sr.Items) == 0 {
			b.WriteString("  (none)\n\n")
			continue
		}
		for _, item := range sr.Items {
			writeScopeItem(&b, item, result)
		}
		b.WriteString("\n")
	}

	// Merged summary
	issueCount, prCount := countMergedTypes(result.Merged)
	b.WriteString(fmt.Sprintf("Merged: %d unique items (%d issues, %d PRs)\n", len(result.Merged), issueCount, prCount))

	_, err := io.WriteString(w, b.String())
	return err
}

func writeScopeItem(b *strings.Builder, item model.DiscoveredItem, result *model.ScopeResult) {
	num := 0
	title := ""
	prefix := "  "

	if item.Issue != nil {
		num = item.Issue.Number
		title = item.Issue.Title
	} else if item.PR != nil {
		num = item.PR.Number
		title = item.PR.Title
		prefix = "  PR "
	}

	if len(title) > 40 {
		title = title[:38] + ".."
	}

	line := fmt.Sprintf("%s#%-5d %s", prefix, num, title)

	// Show if also found by other strategies
	otherStrategies := findOtherStrategies(num, item.Strategy, result)
	if len(otherStrategies) > 0 {
		line += fmt.Sprintf("  (also: %s)", strings.Join(otherStrategies, ", "))
	}

	if item.PR != nil && item.Issue != nil {
		line += fmt.Sprintf("  (PR #%d)", item.PR.Number)
	}

	b.WriteString(line + "\n")
}

func findOtherStrategies(num int, currentStrategy string, result *model.ScopeResult) []string {
	var others []string
	for _, sr := range result.Strategies {
		if sr.Name == currentStrategy {
			continue
		}
		for _, item := range sr.Items {
			itemNum := 0
			if item.Issue != nil {
				itemNum = item.Issue.Number
			} else if item.PR != nil {
				itemNum = item.PR.Number
			}
			if itemNum == num {
				others = append(others, sr.Name)
				break
			}
		}
	}
	return others
}

func countMergedTypes(items []model.DiscoveredItem) (issues, prs int) {
	seenPRs := make(map[int]bool)
	for _, item := range items {
		if item.Issue != nil {
			issues++
		}
		if item.PR != nil && !seenPRs[item.PR.Number] {
			seenPRs[item.PR.Number] = true
			prs++
		}
	}
	return
}

// WriteScopeJSON writes scope results as JSON.
func WriteScopeJSON(w io.Writer, repo string, result *model.ScopeResult) error {
	type jsonItem struct {
		IssueNumber *int   `json:"issue_number,omitempty"`
		IssueTitle  string `json:"issue_title,omitempty"`
		PRNumber    *int   `json:"pr_number,omitempty"`
		PRTitle     string `json:"pr_title,omitempty"`
		Strategy    string `json:"strategy"`
	}

	type jsonStrategyResult struct {
		Name  string     `json:"name"`
		Count int        `json:"count"`
		Items []jsonItem `json:"items"`
	}

	type jsonScopeResult struct {
		Repo        string               `json:"repo"`
		Tag         string               `json:"tag"`
		PreviousTag string               `json:"previous_tag,omitempty"`
		Strategies  []jsonStrategyResult  `json:"strategies"`
		MergedCount int                  `json:"merged_count"`
		Merged      []jsonItem           `json:"merged"`
	}

	convertItem := func(item model.DiscoveredItem) jsonItem {
		ji := jsonItem{Strategy: item.Strategy}
		if item.Issue != nil {
			ji.IssueNumber = &item.Issue.Number
			ji.IssueTitle = item.Issue.Title
		}
		if item.PR != nil {
			ji.PRNumber = &item.PR.Number
			ji.PRTitle = item.PR.Title
		}
		return ji
	}

	out := jsonScopeResult{
		Repo:        repo,
		Tag:         result.Tag,
		PreviousTag: result.PreviousTag,
		MergedCount: len(result.Merged),
	}

	for _, sr := range result.Strategies {
		jsr := jsonStrategyResult{
			Name:  sr.Name,
			Count: len(sr.Items),
		}
		for _, item := range sr.Items {
			jsr.Items = append(jsr.Items, convertItem(item))
		}
		out.Strategies = append(out.Strategies, jsr)
	}

	for _, item := range result.Merged {
		out.Merged = append(out.Merged, convertItem(item))
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteScopeMarkdown writes scope results as markdown.
func WriteScopeMarkdown(w io.Writer, result *model.ScopeResult) error {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Scope: %s", result.Tag))
	if result.PreviousTag != "" {
		b.WriteString(fmt.Sprintf(" (since %s)", result.PreviousTag))
	}
	b.WriteString("\n\n")

	for _, sr := range result.Strategies {
		b.WriteString(fmt.Sprintf("### %s (%d items)\n\n", sr.Name, len(sr.Items)))
		if len(sr.Items) == 0 {
			b.WriteString("_(none)_\n\n")
			continue
		}
		b.WriteString("| # | Title | PR |\n")
		b.WriteString("|---|-------|----|\n")
		for _, item := range sr.Items {
			num := 0
			title := ""
			prRef := ""
			if item.Issue != nil {
				num = item.Issue.Number
				title = item.Issue.Title
			}
			if item.PR != nil {
				if item.Issue == nil {
					num = item.PR.Number
					title = item.PR.Title
				}
				prRef = fmt.Sprintf("#%d", item.PR.Number)
			}
			b.WriteString(fmt.Sprintf("| #%d | %s | %s |\n", num, title, prRef))
		}
		b.WriteString("\n")
	}

	issueCount, prCount := countMergedTypes(result.Merged)
	b.WriteString(fmt.Sprintf("**Merged:** %d unique items (%d issues, %d PRs)\n", len(result.Merged), issueCount, prCount))

	_, err := io.WriteString(w, b.String())
	return err
}
