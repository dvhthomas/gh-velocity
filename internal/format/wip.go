package format

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// --- JSON ---

type jsonWIPOutput struct {
	Repository string        `json:"repository"`
	Items      []jsonWIPItem `json:"items"`
	Count      int           `json:"count"`
}

type jsonWIPItem struct {
	Number           int      `json:"number,omitempty"`
	Title            string   `json:"title"`
	URL              string   `json:"url,omitempty"`
	Labels           []string `json:"labels,omitempty"`
	Status           string   `json:"status"`
	AgeSeconds       int64    `json:"age_seconds"`
	Age              string   `json:"age"`
	Repo             string   `json:"repo,omitempty"`
	Kind             string   `json:"kind"`
	UpdatedAt        string   `json:"updated_at,omitempty"`
	LastActivityDays int      `json:"last_activity_days"`
	Staleness        string   `json:"staleness"`
	Flags            []string `json:"flags,omitempty"`
}

// WriteWIPJSON writes WIP items as JSON.
func WriteWIPJSON(w io.Writer, repo string, items []model.WIPItem) error {
	out := jsonWIPOutput{
		Repository: repo,
		Items:      make([]jsonWIPItem, 0, len(items)),
		Count:      len(items),
	}
	for _, item := range items {
		updatedAtStr := ""
		lastActivityDays := 0
		if !item.UpdatedAt.IsZero() {
			updatedAtStr = item.UpdatedAt.UTC().Format(time.RFC3339)
			lastActivityDays = int(time.Since(item.UpdatedAt).Hours() / 24)
		}
		var flags []string
		switch item.Staleness {
		case model.StalenessStale:
			flags = []string{FlagStale}
		case model.StalenessAging:
			flags = []string{FlagAging}
		}
		out.Items = append(out.Items, jsonWIPItem{
			Number:           item.Number,
			Title:            item.Title,
			URL:              item.URL,
			Labels:           item.Labels,
			Status:           item.Status,
			AgeSeconds:       int64(item.Age.Seconds()),
			Age:              FormatDuration(item.Age),
			Repo:             item.Repo,
			Kind:             item.Kind,
			UpdatedAt:        updatedAtStr,
			LastActivityDays: lastActivityDays,
			Staleness:        string(item.Staleness),
			Flags:            flags,
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
	sorted := SortBy(items, "age", Desc, func(it model.WIPItem) *time.Duration { return &it.Age })

	fmt.Fprintf(rc.Writer, "Work in Progress: %s (%d items)\n\n", repo, len(items))

	if len(sorted.Items) == 0 {
		fmt.Fprintln(rc.Writer, "  No items in progress.")
		return nil
	}

	tp := NewTable(rc.Writer, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"", "#", "Title", "Status", sorted.Header("age", "Age"), "Last Activity"})
	for _, item := range sorted.Items {
		num := ""
		if item.Number > 0 {
			num = FormatItemLink(item.Number, item.URL, rc)
		}
		tp.AddField(wipFlag(item))
		tp.AddField(num)
		tp.AddField(item.Title)
		tp.AddField(item.Status)
		tp.AddField(FormatDuration(item.Age))
		tp.AddField(formatLastActivity(item.UpdatedAt))
		tp.EndRow()
	}
	return tp.Render()
}

// formatLastActivity formats the time since last update as a human-readable string.
func formatLastActivity(updatedAt time.Time) string {
	if updatedAt.IsZero() {
		return "N/A"
	}
	d := time.Since(updatedAt)
	days := int(d.Hours() / 24)
	if days == 0 {
		hours := int(d.Hours())
		if hours == 0 {
			return "just now"
		}
		return fmt.Sprintf("%dh ago", hours)
	}
	return fmt.Sprintf("%dd ago", days)
}

// wipFlag returns an emoji flag based on staleness.
func wipFlag(item model.WIPItem) string {
	switch item.Staleness {
	case model.StalenessStale:
		return FlagEmoji(FlagStale)
	case model.StalenessAging:
		return FlagEmoji(FlagAging)
	default:
		return ""
	}
}
