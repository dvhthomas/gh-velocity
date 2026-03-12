package reviews

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/template"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

//go:embed templates/*.md.tmpl
var templateFS embed.FS

var markdownTmpl = template.Must(
	template.New("reviews.md.tmpl").Funcs(format.TemplateFuncMap()).ParseFS(templateFS, "templates/reviews.md.tmpl"),
)

// --- JSON ---

type jsonOutput struct {
	Repository string     `json:"repository"`
	SearchURL  string     `json:"search_url"`
	Items      []jsonItem `json:"items"`
	Count      int        `json:"count"`
	StaleCount int        `json:"stale_count"`
	Warnings   []string   `json:"warnings,omitempty"`
}

type jsonItem struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url,omitempty"`
	AgeSeconds int64  `json:"age_seconds"`
	Age        string `json:"age"`
	IsStale    bool   `json:"is_stale"`
}

// WriteJSON writes review pressure results as JSON.
func WriteJSON(w io.Writer, result model.ReviewPressureResult, searchURL string, warnings []string) error {
	staleCount := 0
	items := make([]jsonItem, 0, len(result.AwaitingReview))
	for _, pr := range result.AwaitingReview {
		if pr.IsStale {
			staleCount++
		}
		items = append(items, jsonItem{
			Number:     pr.Number,
			Title:      pr.Title,
			URL:        pr.URL,
			AgeSeconds: int64(pr.Age.Seconds()),
			Age:        format.FormatDuration(pr.Age),
			IsStale:    pr.IsStale,
		})
	}
	out := jsonOutput{
		Repository: result.Repository,
		SearchURL:  searchURL,
		Items:      items,
		Count:      len(result.AwaitingReview),
		StaleCount: staleCount,
		Warnings:   warnings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Markdown ---

type templateData struct {
	Repository string
	SearchURL  string
	Items      []reviewItemRow
	Count      int
	StaleCount int
}

type reviewItemRow struct {
	Link   string
	Title  string
	Age    string
	Signal string
}

// WriteMarkdown writes review pressure results as markdown.
func WriteMarkdown(rc format.RenderContext, result model.ReviewPressureResult, searchURL string) error {
	sorted := sortByAgeDesc(result.AwaitingReview)
	data := templateData{
		Repository: result.Repository,
		SearchURL:  searchURL,
		Count:      len(sorted),
	}
	for _, pr := range sorted {
		signal := ""
		if pr.IsStale {
			signal = "STALE"
			data.StaleCount++
		}
		data.Items = append(data.Items, reviewItemRow{
			Link:   format.FormatItemLink(pr.Number, pr.URL, rc),
			Title:  format.SanitizeMarkdown(pr.Title),
			Age:    format.FormatDuration(pr.Age),
			Signal: signal,
		})
	}
	return markdownTmpl.Execute(rc.Writer, data)
}

// --- Pretty ---

// WritePretty writes review pressure results as a formatted table.
func WritePretty(rc format.RenderContext, result model.ReviewPressureResult, searchURL string) error {
	sorted := sortByAgeDesc(result.AwaitingReview)

	fmt.Fprintf(rc.Writer, "Review Queue: %s\n\n", result.Repository)

	if len(sorted) == 0 {
		fmt.Fprintln(rc.Writer, "No PRs currently awaiting review.")
		if searchURL != "" {
			fmt.Fprintf(rc.Writer, "  Verify: %s\n", searchURL)
		}
		return nil
	}

	fmt.Fprintln(rc.Writer, "PRs Awaiting Review (sorted by wait time):")

	tp := format.NewTable(rc.Writer, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"#", "Title", "Age", "Signal"})
	for _, pr := range sorted {
		signal := ""
		if pr.IsStale {
			signal = "STALE"
		}
		tp.AddField(format.FormatItemLink(pr.Number, pr.URL, rc))
		tp.AddField(pr.Title)
		tp.AddField(format.FormatDuration(pr.Age))
		tp.AddField(signal)
		tp.EndRow()
	}
	if err := tp.Render(); err != nil {
		return err
	}

	staleCount := 0
	for _, pr := range sorted {
		if pr.IsStale {
			staleCount++
		}
	}

	fmt.Fprintf(rc.Writer, "\n%d PRs awaiting review", len(sorted))
	if staleCount > 0 {
		fmt.Fprintf(rc.Writer, " (%d stale >48h)", staleCount)
	}
	fmt.Fprintln(rc.Writer)
	return nil
}

// sortByAgeDesc sorts PRs by age descending (longest wait first).
func sortByAgeDesc(items []model.PRAwaitingReview) []model.PRAwaitingReview {
	sorted := make([]model.PRAwaitingReview, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Age > sorted[j].Age
	})
	return sorted
}
