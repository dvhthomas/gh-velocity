package quality

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/template"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

//go:embed templates/*.md.tmpl
var templateFS embed.FS

var qualityMarkdownTmpl = template.Must(
	template.New("quality.md.tmpl").Funcs(format.TemplateFuncMap()).ParseFS(templateFS, "templates/quality.md.tmpl"),
)

// QualityItem holds per-item classification data for the quality detail table.
type QualityItem struct {
	Number   int
	Title    string
	URL      string
	Category string
	LeadTime string // pre-formatted duration
}

// CategoryRow holds a single row of the category breakdown table.
type CategoryRow struct {
	Name  string
	Count int
	Pct   int
}

// Detail holds all data needed to render the quality detail section.
type Detail struct {
	Repository string
	Since      time.Time
	Until      time.Time
	Quality    model.StatsQuality
	Insights   []model.Insight
	Items      []QualityItem
	Categories []CategoryRow
}

// --- JSON ---

type jsonQualityOutput struct {
	Repository string               `json:"repository"`
	Window     format.JSONWindow    `json:"window"`
	Categories []jsonCategory       `json:"categories"`
	Items      []jsonQualityItem    `json:"items"`
	BugCount   int                  `json:"bug_count"`
	TotalIssues int                 `json:"total_issues"`
	BugRatio   float64              `json:"bug_ratio"`
	Insights   []format.JSONInsight `json:"insights,omitempty"`
	Warnings   []string             `json:"warnings,omitempty"`
}

type jsonCategory struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Pct   int    `json:"pct"`
}

type jsonQualityItem struct {
	Number   int      `json:"number"`
	Title    string   `json:"title"`
	URL      string   `json:"url,omitempty"`
	Category string   `json:"category"`
	LeadTime string   `json:"lead_time"`
	Flags    []string `json:"flags,omitempty"`
}

// WriteJSON writes quality detail as JSON.
func WriteJSON(w io.Writer, d Detail, warnings []string) error {
	cats := make([]jsonCategory, 0, len(d.Categories))
	for _, c := range d.Categories {
		cats = append(cats, jsonCategory{Name: c.Name, Count: c.Count, Pct: c.Pct})
	}
	items := make([]jsonQualityItem, 0, len(d.Items))
	for _, item := range d.Items {
		ji := jsonQualityItem{
			Number:   item.Number,
			Title:    item.Title,
			URL:      item.URL,
			Category: item.Category,
			LeadTime: item.LeadTime,
		}
		if item.Category == "bug" {
			ji.Flags = []string{format.FlagBug}
		}
		items = append(items, ji)
	}
	out := jsonQualityOutput{
		Repository: d.Repository,
		Window: format.JSONWindow{
			Since: d.Since.UTC().Format(time.RFC3339),
			Until: d.Until.UTC().Format(time.RFC3339),
		},
		Categories:  cats,
		Items:       items,
		BugCount:    d.Quality.BugCount,
		TotalIssues: d.Quality.TotalIssues,
		BugRatio:    d.Quality.BugRatio,
		Insights:    format.InsightsToJSON(d.Insights),
		Warnings:    warnings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Markdown ---

type templateData struct {
	Repository  string
	Since       time.Time
	Until       time.Time
	Insights    []string
	Categories  []CategoryRow
	BugRatio    int
	BugCount    int
	TotalIssues int
	Items       []itemRow
}

type itemRow struct {
	Flag     string
	Link     string
	Title    string
	Category string
	LeadTime string
}

// WriteMarkdown renders the quality detail section as markdown.
func WriteMarkdown(rc format.RenderContext, d Detail) error {
	data := templateData{
		Repository:  d.Repository,
		Since:       d.Since,
		Until:       d.Until,
		Categories:  d.Categories,
		BugRatio:    int(d.Quality.BugRatio * 100),
		BugCount:    d.Quality.BugCount,
		TotalIssues: d.Quality.TotalIssues,
	}
	for _, ins := range d.Insights {
		data.Insights = append(data.Insights, format.LinkStatTerms(ins.Message))
	}
	for _, item := range d.Items {
		flag := ""
		if item.Category == "bug" {
			flag = format.FlagEmoji(format.FlagBug)
		}
		data.Items = append(data.Items, itemRow{
			Flag:     flag,
			Link:     format.FormatItemLink(item.Number, item.URL, rc),
			Title:    format.SanitizeMarkdown(item.Title),
			Category: item.Category,
			LeadTime: item.LeadTime,
		})
	}
	return qualityMarkdownTmpl.Execute(rc.Writer, data)
}

// --- Pretty ---

// WritePretty renders the quality detail section as formatted text.
func WritePretty(rc format.RenderContext, d Detail) error {
	fmt.Fprintf(rc.Writer, "Quality: %s (%s – %s UTC)\n\n",
		d.Repository, d.Since.UTC().Format(time.DateOnly), d.Until.UTC().Format(time.DateOnly))

	model.WriteInsightsPretty(rc.Writer, d.Insights)

	fmt.Fprintln(rc.Writer, "  Category Breakdown:")
	for _, cat := range d.Categories {
		fmt.Fprintf(rc.Writer, "    %-20s %3d  (%d%%)\n", cat.Name, cat.Count, cat.Pct)
	}
	fmt.Fprintf(rc.Writer, "\n  Bug ratio: %d%% (%d bugs / %d issues)\n\n",
		int(d.Quality.BugRatio*100), d.Quality.BugCount, d.Quality.TotalIssues)

	if len(d.Items) > 0 {
		tp := format.NewTable(rc.Writer, rc.IsTTY, rc.Width)
		tp.AddHeader([]string{"", "#", "Title", "Category", "Lead Time"})
		for _, item := range d.Items {
			flag := ""
			if item.Category == "bug" {
				flag = format.FlagEmoji(format.FlagBug)
			}
			tp.AddField(flag)
			tp.AddField(format.FormatItemLink(item.Number, item.URL, rc))
			tp.AddField(item.Title)
			tp.AddField(item.Category)
			tp.AddField(item.LeadTime)
			tp.EndRow()
		}
		return tp.Render()
	}
	return nil
}

// BuildCategories computes the category breakdown from per-item data.
func BuildCategories(items []QualityItem) []CategoryRow {
	counts := make(map[string]int)
	total := len(items)
	for _, item := range items {
		cat := item.Category
		if cat == "" {
			cat = "other"
		}
		counts[cat]++
	}
	rows := make([]CategoryRow, 0, len(counts))
	for name, count := range counts {
		pct := 0
		if total > 0 {
			pct = count * 100 / total
		}
		rows = append(rows, CategoryRow{Name: name, Count: count, Pct: pct})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}
