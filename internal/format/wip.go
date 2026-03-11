package format

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// --- JSON ---

type jsonWIPOutput struct {
	Repository string        `json:"repository"`
	Items      []jsonWIPItem `json:"items"`
	Count      int           `json:"count"`
}

type jsonWIPItem struct {
	Number     int      `json:"number,omitempty"`
	Title      string   `json:"title"`
	URL        string   `json:"url,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	Status     string   `json:"status"`
	AgeSeconds int64    `json:"age_seconds"`
	Age        string   `json:"age"`
	Repo       string   `json:"repo,omitempty"`
	Kind       string   `json:"kind"`
}

// WriteWIPJSON writes WIP items as JSON.
func WriteWIPJSON(w io.Writer, repo string, items []model.WIPItem) error {
	out := jsonWIPOutput{
		Repository: repo,
		Items:      make([]jsonWIPItem, 0, len(items)),
		Count:      len(items),
	}
	for _, item := range items {
		out.Items = append(out.Items, jsonWIPItem{
			Number:     item.Number,
			Title:      item.Title,
			URL:        item.URL,
			Labels:     item.Labels,
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

// WriteWIPMarkdown writes WIP items as a markdown table using an embedded template.
func WriteWIPMarkdown(rc RenderContext, repo string, items []model.WIPItem) error {
	return renderWIPMarkdown(rc.Writer, rc, repo, items)
}

// --- Pretty ---

// WriteWIPPretty writes WIP items as a formatted table.
func WriteWIPPretty(rc RenderContext, repo string, items []model.WIPItem) error {
	sorted := sortWIPByAgeDesc(items)

	fmt.Fprintf(rc.Writer, "Work in Progress: %s (%d items)\n\n", repo, len(items))

	if len(sorted) == 0 {
		fmt.Fprintln(rc.Writer, "  No items in progress.")
		return nil
	}

	tp := NewTable(rc.Writer, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"#", "Title", "Labels", "Status", "Age", "Kind"})
	for _, item := range sorted {
		num := ""
		if item.Number > 0 {
			num = FormatItemLink(item.Number, item.URL, rc)
		}
		tp.AddField(num)
		tp.AddField(item.Title)
		tp.AddField(FormatLabels(item.Labels))
		tp.AddField(item.Status)
		tp.AddField(FormatDuration(item.Age))
		tp.AddField(item.Kind)
		tp.EndRow()
	}
	return tp.Render()
}

// sortWIPByAgeDesc sorts WIP items by age descending (oldest first).
func sortWIPByAgeDesc(items []model.WIPItem) []model.WIPItem {
	sorted := make([]model.WIPItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Age > sorted[j].Age
	})
	return sorted
}
