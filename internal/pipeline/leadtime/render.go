package leadtime

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/template"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

const (
	noiseThreshold  = time.Minute    // items resolved faster than this are likely noise/automation
	hotfixThreshold = 72 * time.Hour // items resolved within this window are hotfixes
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
	Repository string            `json:"repository"`
	Issue      int               `json:"issue"`
	Title      string            `json:"title"`
	State      string            `json:"state"`
	URL        string            `json:"url,omitempty"`
	Labels     []string          `json:"labels,omitempty"`
	LeadTime   format.JSONMetric `json:"lead_time"`
	Warnings   []string          `json:"warnings,omitempty"`
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
	Repository string               `json:"repository"`
	Window     format.JSONWindow    `json:"window"`
	SearchURL  string               `json:"search_url"`
	Insights   []format.JSONInsight `json:"insights,omitempty"`
	Items      []jsonBulkItem       `json:"items"`
	Stats      format.JSONStats     `json:"stats"`
	Capped     bool                 `json:"capped,omitempty"`
	Warnings   []string             `json:"warnings,omitempty"`
}

type jsonBulkItem struct {
	Number   int               `json:"number"`
	Title    string            `json:"title"`
	URL      string            `json:"url,omitempty"`
	Labels   []string          `json:"labels,omitempty"`
	LeadTime format.JSONMetric `json:"lead_time"`
	Flags    []string          `json:"flags,omitempty"`
}

// WriteBulkJSON writes bulk lead-time results as JSON.
func WriteBulkJSON(w io.Writer, repo string, since, until time.Time, items []BulkItem, stats model.Stats, searchURL string, warnings []string, insights []model.Insight) error {
	jsonIns := format.InsightsToJSON(insights)
	out := jsonBulkOutput{
		Repository: repo,
		Window: format.JSONWindow{
			Since: since.UTC().Format(time.RFC3339),
			Until: until.UTC().Format(time.RFC3339),
		},
		SearchURL: searchURL,
		Insights:  jsonIns,
		Items:     make([]jsonBulkItem, 0, len(items)),
		Stats:     format.StatsToJSON(stats),
		Capped:    len(items) >= 1000,
		Warnings:  warnings,
	}

	for _, item := range items {
		ji := jsonBulkItem{
			Number:   item.Issue.Number,
			Title:    item.Issue.Title,
			URL:      item.Issue.URL,
			Labels:   item.Issue.Labels,
			LeadTime: format.MetricToJSON(item.Metric),
		}
		if item.Metric.Duration != nil && *item.Metric.Duration < noiseThreshold {
			ji.Flags = append(ji.Flags, "noise")
		}
		if item.Metric.Duration != nil && *item.Metric.Duration <= hotfixThreshold && *item.Metric.Duration >= noiseThreshold {
			ji.Flags = append(ji.Flags, "hotfix")
		}
		if metrics.IsOutlier(item.Metric, stats) {
			ji.Flags = append(ji.Flags, "outlier")
		}
		out.Items = append(out.Items, ji)
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
	Insights   []string
	Items      []bulkItemRow
	Detail     []string
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
	Flag     string // e.g., "🚩" for outliers, empty otherwise
}

// WriteBulkMarkdown writes bulk lead-time results as markdown.
func WriteBulkMarkdown(rc format.RenderContext, repo string, since, until time.Time, items []BulkItem, stats model.Stats, searchURL string, insights []model.Insight) error {
	sorted := sortByCloseDateDesc(items)
	var insightMsgs []string
	for _, ins := range insights {
		insightMsgs = append(insightMsgs, format.LinkStatTerms(ins.Message))
	}
	data := bulkTemplateData{
		Repository: repo,
		Since:      since,
		Until:      until,
		Insights:   insightMsgs,
		Detail:     format.FormatStatsDetail(stats),
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
			Flag:     leadTimeFlag(item, stats),
		})
	}
	return bulkMarkdownTmpl.Execute(rc.Writer, data)
}

// ============================================================
// Bulk Pretty
// ============================================================

// WriteBulkPretty writes bulk lead-time results as a formatted table.
func WriteBulkPretty(rc format.RenderContext, repo string, since, until time.Time, items []BulkItem, stats model.Stats, searchURL string, insights []model.Insight) error {
	sorted := sortByCloseDateDesc(items)

	fmt.Fprintf(rc.Writer, "Lead Time: %s (%s – %s UTC)\n\n",
		repo, since.UTC().Format(time.DateOnly), until.UTC().Format(time.DateOnly))
	model.WriteInsightsPretty(rc.Writer, insights)
	fmt.Fprintln(rc.Writer, "  Summary:")
	for _, line := range format.FormatStatsDetail(stats) {
		fmt.Fprintf(rc.Writer, "    %s\n", format.StripMarkdownLinks(format.StripMarkdownBold(line)))
	}
	fmt.Fprintln(rc.Writer)

	if len(sorted) == 0 {
		fmt.Fprintln(rc.Writer, "  No issues closed in this period.")
		if searchURL != "" {
			fmt.Fprintf(rc.Writer, "  Verify: %s\n", searchURL)
		}
		return nil
	}

	tp := format.NewTable(rc.Writer, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"", "#", "Title", "Labels", "Created", "Closed", "Lead Time"})
	for _, item := range sorted {
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		tp.AddField(leadTimeFlag(item, stats))
		tp.AddField(format.FormatItemLink(item.Issue.Number, item.Issue.URL, rc))
		tp.AddField(item.Issue.Title)
		tp.AddField(format.FormatLabels(item.Issue.Labels))
		tp.AddField(item.Issue.CreatedAt.UTC().Format(time.DateOnly))
		tp.AddField(closedStr)
		tp.AddField(format.FormatMetricDuration(item.Metric))
		tp.EndRow()
	}
	return tp.Render()
}

// leadTimeFlag returns a flag emoji for an item based on insight-triggering conditions.
// Multiple flags can apply — they're concatenated.
func leadTimeFlag(item BulkItem, stats model.Stats) string {
	var flag string
	// Noise: resolved in under 60 seconds (likely bot/automation).
	if item.Metric.Duration != nil && *item.Metric.Duration < noiseThreshold {
		flag += "🤖"
	}
	// Hotfix: resolved within 72 hours of creation.
	if item.Metric.Duration != nil && *item.Metric.Duration <= hotfixThreshold && *item.Metric.Duration >= noiseThreshold {
		flag += "⚡"
	}
	// Outlier: duration exceeds IQR cutoff.
	if metrics.IsOutlier(item.Metric, stats) {
		flag += "🚩"
	}
	return flag
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
