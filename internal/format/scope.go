package format

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// WriteScopePretty writes scope results as a formatted table to the writer.
func WriteScopePretty(w io.Writer, isTTY bool, width int, result *model.ScopeResult) error {
	fmt.Fprintf(w, "Scope: %s", result.Tag)
	if result.PreviousTag != "" {
		fmt.Fprintf(w, " (since %s)", result.PreviousTag)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.Repeat("=", 60))
	fmt.Fprintln(w)

	// Per-strategy breakdown
	for _, sr := range result.Strategies {
		fmt.Fprintf(w, "Strategy: %s (%d items)\n", sr.Name, len(sr.Items))
		if len(sr.Items) == 0 {
			fmt.Fprintln(w, "  (none)")
			fmt.Fprintln(w)
			continue
		}
		tp := NewTable(w, isTTY, width)
		tp.AddHeader([]string{"#", "Title", "Linked"})
		for _, item := range sr.Items {
			addScopeItemRow(tp, item, result)
		}
		if err := tp.Render(); err != nil {
			return err
		}
		fmt.Fprintln(w)
	}

	// Merged summary
	issueCount, prCount := countMergedTypes(result.Merged)
	fmt.Fprintf(w, "Merged: %d unique items (%d issues, %d PRs)\n", len(result.Merged), issueCount, prCount)

	return nil
}

func addScopeItemRow(tp tableprinter.TablePrinter, item model.DiscoveredItem, result *model.ScopeResult) {
	num := 0
	title := ""
	prefix := ""

	if item.Issue != nil {
		num = item.Issue.Number
		title = item.Issue.Title
	} else if item.PR != nil {
		num = item.PR.Number
		title = item.PR.Title
		prefix = "PR "
	}

	ref := fmt.Sprintf("%s#%d", prefix, num)

	// Show linked PR if this is an issue
	extra := ""
	if item.PR != nil && item.Issue != nil {
		extra = fmt.Sprintf("PR #%d", item.PR.Number)
	}

	// Show if also found by other strategies
	otherStrategies := findOtherStrategies(num, item.Strategy, result)
	if len(otherStrategies) > 0 {
		if extra != "" {
			extra += "  "
		}
		extra += fmt.Sprintf("(also: %s)", strings.Join(otherStrategies, ", "))
	}

	tp.AddField(ref)
	tp.AddField(title)
	tp.AddField(extra)
	tp.EndRow()
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
		Strategies  []jsonStrategyResult `json:"strategies"`
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

// WriteScopeMarkdown writes scope results as markdown using an embedded template.
func WriteScopeMarkdown(w io.Writer, result *model.ScopeResult) error {
	return renderScopeMarkdown(w, result)
}
