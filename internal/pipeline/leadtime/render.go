package leadtime

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/template"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

//go:embed templates/*.md.tmpl
var templateFS embed.FS

var bulkMarkdownTmpl = template.Must(
	template.New("leadtime-bulk.md.tmpl").Funcs(format.TemplateFuncMap()).ParseFS(templateFS, "templates/leadtime-bulk.md.tmpl"),
)

// ============================================================
// Single Issue JSON
// ============================================================

type jsonSingleOutput struct {
	Repository string          `json:"repository"`
	Issue      int             `json:"issue"`
	Title      string          `json:"title"`
	State      string          `json:"state"`
	URL        string          `json:"url,omitempty"`
	Labels     []string        `json:"labels,omitempty"`
	LeadTime   format.JSONMetric `json:"lead_time"`
	Warnings   []string        `json:"warnings,omitempty"`
}

// WriteSingleJSON writes lead-time metrics for a single issue as JSON.
func WriteSingleJSON(w io.Writer, repo string, issueNumber int, title, state, issueURL string, labels []string, m model.Metric, warnings []string) error {
	out := jsonSingleOutput{
		Repository: repo,
		Issue:      issueNumber,
		Title:      title,
		State:      state,
		URL:        issueURL,
		Labels:     labels,
		LeadTime:   format.MetricToJSON(m),
		Warnings:   warnings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ============================================================
// Bulk JSON
// ============================================================

type jsonBulkOutput struct {
	Repository string         `json:"repository"`
	Window     format.JSONWindow `json:"window"`
	SearchURL  string         `json:"search_url"`
	Items      []jsonBulkItem `json:"items"`
	Stats      format.JSONStats  `json:"stats"`
	Capped     bool           `json:"capped,omitempty"`
}

type jsonBulkItem struct {
	Number   int             `json:"number"`
	Title    string          `json:"title"`
	URL      string          `json:"url,omitempty"`
	Labels   []string        `json:"labels,omitempty"`
	LeadTime format.JSONMetric `json:"lead_time"`
}

// WriteBulkJSON writes bulk lead-time results as JSON.
func WriteBulkJSON(w io.Writer, repo string, since, until time.Time, items []BulkItem, stats model.Stats, searchURL string) error {
	out := jsonBulkOutput{
		Repository: repo,
		Window: format.JSONWindow{
			Since: since.UTC().Format(time.RFC3339),
			Until: until.UTC().Format(time.RFC3339),
		},
		SearchURL: searchURL,
		Items:     make([]jsonBulkItem, 0, len(items)),
		Stats:     format.StatsToJSON(stats),
		Capped:    len(items) >= 1000,
	}

	for _, item := range items {
		out.Items = append(out.Items, jsonBulkItem{
			Number:   item.Issue.Number,
			Title:    item.Issue.Title,
			URL:      item.Issue.URL,
			Labels:   item.Issue.Labels,
			LeadTime: format.MetricToJSON(item.Metric),
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ============================================================
// Bulk Markdown
// ============================================================

type bulkTemplateData struct {
	Repository string
	Since      time.Time
	Until      time.Time
	Items      []bulkItemRow
	Summary    string
	SearchURL  string
}

type bulkItemRow struct {
	Link     string
	Title    string
	Labels   string
	Created  string
	Closed   string
	LeadTime string
}

// WriteBulkMarkdown writes bulk lead-time results as markdown.
func WriteBulkMarkdown(rc format.RenderContext, repo string, since, until time.Time, items []BulkItem, stats model.Stats, searchURL string) error {
	sorted := sortByCloseDateDesc(items)
	data := bulkTemplateData{
		Repository: repo,
		Since:      since,
		Until:      until,
		Summary:    format.FormatStatsSummary(stats),
		SearchURL:  searchURL,
	}
	for _, item := range sorted {
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		data.Items = append(data.Items, bulkItemRow{
			Link:     format.FormatItemLink(item.Issue.Number, item.Issue.URL, rc),
			Title:    format.SanitizeMarkdown(item.Issue.Title),
			Labels:   format.FormatLabels(item.Issue.Labels),
			Created:  item.Issue.CreatedAt.UTC().Format(time.DateOnly),
			Closed:   closedStr,
			LeadTime: format.FormatMetricDuration(item.Metric),
		})
	}
	return bulkMarkdownTmpl.Execute(rc.Writer, data)
}

// ============================================================
// Bulk Pretty
// ============================================================

// WriteBulkPretty writes bulk lead-time results as a formatted table.
func WriteBulkPretty(rc format.RenderContext, repo string, since, until time.Time, items []BulkItem, stats model.Stats, searchURL string) error {
	sorted := sortByCloseDateDesc(items)

	fmt.Fprintf(rc.Writer, "Lead Time: %s (%s – %s UTC)\n\n",
		repo, since.UTC().Format(time.DateOnly), until.UTC().Format(time.DateOnly))

	if len(sorted) == 0 {
		fmt.Fprintln(rc.Writer, "  No issues closed in this period.")
		if searchURL != "" {
			fmt.Fprintf(rc.Writer, "  Verify: %s\n", searchURL)
		}
		return nil
	}

	tp := format.NewTable(rc.Writer, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"#", "Title", "Labels", "Created", "Closed", "Lead Time"})
	for _, item := range sorted {
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		tp.AddField(format.FormatItemLink(item.Issue.Number, item.Issue.URL, rc))
		tp.AddField(item.Issue.Title)
		tp.AddField(format.FormatLabels(item.Issue.Labels))
		tp.AddField(item.Issue.CreatedAt.UTC().Format(time.DateOnly))
		tp.AddField(closedStr)
		tp.AddField(format.FormatMetricDuration(item.Metric))
		tp.EndRow()
	}
	if err := tp.Render(); err != nil {
		return err
	}

	fmt.Fprintf(rc.Writer, "\n%s\n", format.FormatStatsSummary(stats))
	return nil
}

// --- Helpers ---

func sortByCloseDateDesc(items []BulkItem) []BulkItem {
	sorted := make([]BulkItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		ci := sorted[i].Issue.ClosedAt
		cj := sorted[j].Issue.ClosedAt
		if ci == nil && cj == nil {
			return false
		}
		if ci == nil {
			return false
		}
		if cj == nil {
			return true
		}
		return ci.After(*cj)
	})
	return sorted
}
