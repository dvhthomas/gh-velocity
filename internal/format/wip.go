package format

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// WIPItem is the format-layer representation of an in-progress work item.
// Imported by cmd/wip.go which populates these from API results.
type WIPItem struct {
	Number int
	Title  string
	Status string
	Age    time.Duration
	Repo   string // populated for cross-repo board views
	Kind   string // "Issue", "PullRequest", "DraftIssue"
}

// --- JSON ---

type jsonWIPOutput struct {
	Repository string        `json:"repository"`
	Items      []jsonWIPItem `json:"items"`
	Count      int           `json:"count"`
}

type jsonWIPItem struct {
	Number     int    `json:"number,omitempty"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	AgeSeconds int64  `json:"age_seconds"`
	Age        string `json:"age"`
	Repo       string `json:"repo,omitempty"`
	Kind       string `json:"kind"`
}

// WriteWIPJSON writes WIP items as JSON.
func WriteWIPJSON(w io.Writer, repo string, items []WIPItem) error {
	out := jsonWIPOutput{
		Repository: repo,
		Items:      make([]jsonWIPItem, 0, len(items)),
		Count:      len(items),
	}
	for _, item := range items {
		out.Items = append(out.Items, jsonWIPItem{
			Number:     item.Number,
			Title:      item.Title,
			Status:     item.Status,
			AgeSeconds: int64(item.Age.Seconds()),
			Age:        FormatDuration(item.Age),
			Repo:       item.Repo,
			Kind:       item.Kind,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Markdown ---

// WriteWIPMarkdown writes WIP items as a markdown table.
func WriteWIPMarkdown(w io.Writer, repo string, items []WIPItem) error {
	sorted := sortWIPByAgeDesc(items)

	fmt.Fprintf(w, "## Work in Progress: %s\n\n", repo)
	fmt.Fprintf(w, "| # | Title | Status | Age | Kind |\n")
	fmt.Fprintf(w, "| ---: | --- | --- | --- | --- |\n")
	for _, item := range sorted {
		num := ""
		if item.Number > 0 {
			num = fmt.Sprintf("#%d", item.Number)
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n",
			num,
			sanitizeMarkdown(item.Title),
			item.Status,
			FormatDuration(item.Age),
			item.Kind,
		)
	}
	fmt.Fprintf(w, "\n**Count:** %d\n", len(items))
	return nil
}

// --- Pretty ---

// WriteWIPPretty writes WIP items as a formatted table.
func WriteWIPPretty(w io.Writer, isTTY bool, width int, repo string, items []WIPItem) error {
	sorted := sortWIPByAgeDesc(items)

	fmt.Fprintf(w, "Work in Progress: %s (%d items)\n\n", repo, len(items))

	if len(sorted) == 0 {
		fmt.Fprintln(w, "  No items in progress.")
		return nil
	}

	tp := NewTable(w, isTTY, width)
	tp.AddHeader([]string{"#", "Title", "Status", "Age", "Kind"})
	for _, item := range sorted {
		num := ""
		if item.Number > 0 {
			num = fmt.Sprintf("%d", item.Number)
		}
		tp.AddField(num)
		tp.AddField(item.Title)
		tp.AddField(item.Status)
		tp.AddField(FormatDuration(item.Age))
		tp.AddField(item.Kind)
		tp.EndRow()
	}
	return tp.Render()
}

// sortWIPByAgeDesc sorts WIP items by age descending (oldest first).
func sortWIPByAgeDesc(items []WIPItem) []WIPItem {
	sorted := make([]WIPItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Age > sorted[j].Age
	})
	return sorted
}
