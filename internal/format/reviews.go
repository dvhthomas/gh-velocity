package format

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// --- JSON ---

type jsonReviewsOutput struct {
	Repository string           `json:"repository"`
	SearchURL  string           `json:"search_url"`
	Items      []jsonReviewItem `json:"items"`
	Count      int              `json:"count"`
	StaleCount int              `json:"stale_count"`
}

type jsonReviewItem struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url,omitempty"`
	AgeSeconds int64  `json:"age_seconds"`
	Age        string `json:"age"`
	IsStale    bool   `json:"is_stale"`
}

// WriteReviewsJSON writes review pressure results as JSON.
func WriteReviewsJSON(w io.Writer, result model.ReviewPressureResult, searchURL string) error {
	staleCount := 0
	items := make([]jsonReviewItem, 0, len(result.AwaitingReview))
	for _, pr := range result.AwaitingReview {
		if pr.IsStale {
			staleCount++
		}
		items = append(items, jsonReviewItem{
			Number:     pr.Number,
			Title:      pr.Title,
			URL:        pr.URL,
			AgeSeconds: int64(pr.Age.Seconds()),
			Age:        FormatDuration(pr.Age),
			IsStale:    pr.IsStale,
		})
	}
	out := jsonReviewsOutput{
		Repository: result.Repository,
		SearchURL:  searchURL,
		Items:      items,
		Count:      len(result.AwaitingReview),
		StaleCount: staleCount,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Markdown ---

// WriteReviewsMarkdown writes review pressure results as markdown using an embedded template.
func WriteReviewsMarkdown(rc RenderContext, result model.ReviewPressureResult, searchURL string) error {
	return renderReviewsMarkdown(rc.Writer, rc, result, searchURL)
}

// --- Pretty ---

// WriteReviewsPretty writes review pressure results as a formatted table.
func WriteReviewsPretty(rc RenderContext, result model.ReviewPressureResult, searchURL string) error {
	sorted := sortReviewsByAgeDesc(result.AwaitingReview)

	fmt.Fprintf(rc.Writer, "Review Queue: %s\n\n", result.Repository)

	if len(sorted) == 0 {
		fmt.Fprintln(rc.Writer, "No PRs currently awaiting review.")
		if searchURL != "" {
			fmt.Fprintf(rc.Writer, "  Verify: %s\n", searchURL)
		}
		return nil
	}

	fmt.Fprintln(rc.Writer, "PRs Awaiting Review (sorted by wait time):")

	tp := NewTable(rc.Writer, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"#", "Title", "Age", "Signal"})
	for _, pr := range sorted {
		signal := ""
		if pr.IsStale {
			signal = "STALE"
		}
		tp.AddField(FormatItemLink(pr.Number, pr.URL, rc))
		tp.AddField(pr.Title)
		tp.AddField(FormatDuration(pr.Age))
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

// sortReviewsByAgeDesc sorts PRs by age descending (longest wait first).
func sortReviewsByAgeDesc(items []model.PRAwaitingReview) []model.PRAwaitingReview {
	sorted := make([]model.PRAwaitingReview, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Age > sorted[j].Age
	})
	return sorted
}
