package reviews

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"text/template"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

//go:embed templates/*.md.tmpl
var templateFS embed.FS

var markdownTmpl = template.Must(
	template.New("reviews.md.tmpl").Funcs(format.TemplateFuncMap()).ParseFS(templateFS, "templates/reviews.md.tmpl"),
)

// --- JSON ---

type jsonOutput struct {
	Repository string               `json:"repository"`
	SearchURL  string               `json:"search_url"`
	Sort       format.JSONSort      `json:"sort"`
	Insights   []format.JSONInsight `json:"insights,omitempty"`
	Items      []jsonItem           `json:"items"`
	Count      int                  `json:"count"`
	StaleCount int                  `json:"stale_count"`
	Warnings   []string             `json:"warnings,omitempty"`
}

type jsonItem struct {
	Number     int      `json:"number"`
	Title      string   `json:"title"`
	URL        string   `json:"url,omitempty"`
	AgeSeconds int64    `json:"age_seconds"`
	Age        string   `json:"age"`
	Flags      []string `json:"flags,omitempty"`
}

// WriteJSON writes review pressure results as JSON.
func WriteJSON(w io.Writer, result model.ReviewPressureResult, searchURL string, warnings []string) error {
	sorted := format.SortBy(result.AwaitingReview, "age", format.Desc, func(pr model.PRAwaitingReview) *time.Duration { return &pr.Age })
	staleCount := 0
	items := make([]jsonItem, 0, len(sorted.Items))
	for _, pr := range sorted.Items {
		flags := reviewFlags(pr)
		if pr.IsStale {
			staleCount++
		}
		items = append(items, jsonItem{
			Number:     pr.Number,
			Title:      pr.Title,
			URL:        pr.URL,
			AgeSeconds: int64(pr.Age.Seconds()),
			Age:        format.FormatDuration(pr.Age),
			Flags:      flags,
		})
	}
	out := jsonOutput{
		Repository: result.Repository,
		SearchURL:  searchURL,
		Sort:       sorted.JSONSort(),
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
	SortHeader string
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
	sorted := format.SortBy(result.AwaitingReview, "age", format.Desc, func(pr model.PRAwaitingReview) *time.Duration { return &pr.Age })
	data := templateData{
		Repository: result.Repository,
		SearchURL:  searchURL,
		SortHeader: sorted.Header("age", "Age"),
		Count:      len(sorted.Items),
	}
	for _, pr := range sorted.Items {
		signal := flagEmojis(reviewFlags(pr))
		if pr.IsStale {
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
	sorted := format.SortBy(result.AwaitingReview, "age", format.Desc, func(pr model.PRAwaitingReview) *time.Duration { return &pr.Age })

	fmt.Fprintf(rc.Writer, "Review Queue: %s\n\n", result.Repository)

	if len(sorted.Items) == 0 {
		fmt.Fprintln(rc.Writer, "No PRs currently awaiting review.")
		if searchURL != "" {
			fmt.Fprintf(rc.Writer, "  Verify: %s\n", searchURL)
		}
		return nil
	}

	fmt.Fprintln(rc.Writer, "PRs Awaiting Review:")

	tp := format.NewTable(rc.Writer, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"", "#", "Title", sorted.Header("age", "Age")})
	for _, pr := range sorted.Items {
		tp.AddField(flagEmojis(reviewFlags(pr)))
		tp.AddField(format.FormatItemLink(pr.Number, pr.URL, rc))
		tp.AddField(pr.Title)
		tp.AddField(format.FormatDuration(pr.Age))
		tp.EndRow()
	}
	if err := tp.Render(); err != nil {
		return err
	}

	staleCount := 0
	for _, pr := range sorted.Items {
		if pr.IsStale {
			staleCount++
		}
	}

	fmt.Fprintf(rc.Writer, "\n%d PRs awaiting review", len(sorted.Items))
	if staleCount > 0 {
		fmt.Fprintf(rc.Writer, " (%d stale >48h)", staleCount)
	}
	fmt.Fprintln(rc.Writer)
	return nil
}

// reviewFlags returns the applicable flag constants for a PR.
func reviewFlags(pr model.PRAwaitingReview) []string {
	if pr.IsStale {
		return []string{format.FlagStale}
	}
	// Aging: >24h but not yet stale (>48h).
	if pr.Age > 24*time.Hour {
		return []string{format.FlagAging}
	}
	return nil
}

// flagEmojis concatenates emoji for a set of flags.
func flagEmojis(flags []string) string {
	var s string
	for _, f := range flags {
		s += format.FlagEmoji(f)
	}
	return s
}
