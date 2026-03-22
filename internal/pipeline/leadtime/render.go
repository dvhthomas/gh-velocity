package leadtime

import (
	"embed"
	"fmt"
	"io"
	"text/template"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
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
	return format.WriteIndentedJSON(w, jsonSingleOutput{
		Repository: repo,
		Issue:      issueNumber,
		Title:      title,
		State:      state,
		URL:        issueURL,
		Labels:     labels,
		LeadTime:   format.MetricToJSON(m),
		Warnings:   warnings,
	})
}

// ============================================================
// Bulk JSON
// ============================================================

type jsonBulkOutput struct {
	format.BulkEnvelope
	Items []jsonBulkItem `json:"items"`
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
	sorted := format.SortBy(items, "lead_time", format.Desc, func(it BulkItem) *time.Duration { return it.Metric.Duration })
	out := jsonBulkOutput{
		BulkEnvelope: format.BulkEnvelope{
			Repository: repo,
			Window: format.JSONWindow{
				Since: since.UTC().Format(time.RFC3339),
				Until: until.UTC().Format(time.RFC3339),
			},
			SearchURL: searchURL,
			Sort:      sorted.JSONSort(),
			Insights:  format.InsightsToJSON(insights),
			Stats:     format.StatsToJSON(stats),
			Capped:    len(items) >= 1000,
			Warnings:  warnings,
		},
		Items: make([]jsonBulkItem, 0, len(sorted.Items)),
	}

	for _, item := range sorted.Items {
		out.Items = append(out.Items, jsonBulkItem{
			Number:   item.Issue.Number,
			Title:    item.Issue.Title,
			URL:      item.Issue.URL,
			Labels:   item.Issue.Labels,
			LeadTime: format.MetricToJSON(item.Metric),
			Flags:    classifyFlags(item, stats),
		})
	}

	return format.WriteIndentedJSON(w, out)
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
	SortHeader string // e.g. "Lead Time ↓"
	TotalCount int    // total items before capping
	Capped     bool   // true when items were truncated
}

type bulkItemRow struct {
	Link     string
	Title    string
	Closed   string
	LeadTime string
	Flag     string // e.g., "🚩" for outliers, empty otherwise
}

// WriteBulkMarkdown writes bulk lead-time results as markdown.
func WriteBulkMarkdown(rc format.RenderContext, repo string, since, until time.Time, items []BulkItem, stats model.Stats, searchURL string, insights []model.Insight) error {
	sorted := format.SortBy(items, "lead_time", format.Desc, func(it BulkItem) *time.Duration { return it.Metric.Duration })
	var insightMsgs []string
	for _, ins := range insights {
		insightMsgs = append(insightMsgs, format.LinkStatTerms(ins.Message))
	}
	total := len(sorted.Items)
	capped := total > format.MaxDetailRows
	renderItems := sorted.Items
	if capped {
		renderItems = renderItems[:format.MaxDetailRows]
	}
	data := bulkTemplateData{
		Repository: repo,
		Since:      since,
		Until:      until,
		Insights:   insightMsgs,
		Detail:     format.FormatStatsDetail(stats),
		Summary:    format.FormatStatsSummary(stats),
		SearchURL:  searchURL,
		SortHeader: sorted.Header("lead_time", "Lead Time"),
		TotalCount: total,
		Capped:     capped,
	}
	for _, item := range renderItems {
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		data.Items = append(data.Items, bulkItemRow{
			Link:     format.FormatItemLink(item.Issue.Number, item.Issue.URL, rc),
			Title:    format.SanitizeMarkdown(item.Issue.Title),
			Closed:   closedStr,
			LeadTime: format.FormatMetricDuration(item.Metric),
			Flag:     flagEmojis(classifyFlags(item, stats)),
		})
	}
	return bulkMarkdownTmpl.Execute(rc.Writer, data)
}

// ============================================================
// Bulk Pretty
// ============================================================

// WriteBulkPretty writes bulk lead-time results as a formatted table.
func WriteBulkPretty(rc format.RenderContext, repo string, since, until time.Time, items []BulkItem, stats model.Stats, searchURL string, insights []model.Insight) error {
	sorted := format.SortBy(items, "lead_time", format.Desc, func(it BulkItem) *time.Duration { return it.Metric.Duration })

	fmt.Fprintf(rc.Writer, "Lead Time: %s (%s – %s UTC)\n\n",
		repo, since.UTC().Format(time.DateOnly), until.UTC().Format(time.DateOnly))
	model.WriteInsightsPretty(rc.Writer, insights)
	fmt.Fprintln(rc.Writer, "  Summary:")
	for _, line := range format.FormatStatsDetail(stats) {
		fmt.Fprintf(rc.Writer, "    %s\n", format.StripMarkdownLinks(format.StripMarkdownBold(line)))
	}
	fmt.Fprintln(rc.Writer)

	if len(sorted.Items) == 0 {
		fmt.Fprintln(rc.Writer, "  No issues closed in this period.")
		if searchURL != "" {
			fmt.Fprintf(rc.Writer, "  Verify: %s\n", searchURL)
		}
		return nil
	}

	total := len(sorted.Items)
	capped := total > format.MaxDetailRows
	renderItems := sorted.Items
	if capped {
		renderItems = renderItems[:format.MaxDetailRows]
	}

	tp := format.NewTable(rc.Writer, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"", "#", "Title", "Closed", sorted.Header("lead_time", "Lead Time")})
	for _, item := range renderItems {
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		tp.AddField(flagEmojis(classifyFlags(item, stats)))
		tp.AddField(format.FormatItemLink(item.Issue.Number, item.Issue.URL, rc))
		tp.AddField(item.Issue.Title)
		tp.AddField(closedStr)
		tp.AddField(format.FormatMetricDuration(item.Metric))
		tp.EndRow()
	}
	if err := tp.Render(); err != nil {
		return err
	}
	if capped {
		fmt.Fprintf(rc.Writer, "  %d more items not shown. Use --format json for complete data.\n", total-format.MaxDetailRows)
	}
	return nil
}

// classifyFlags returns the applicable flag constants for a duration-based item.
func classifyFlags(item BulkItem, stats model.Stats) []string {
	return format.ClassifyDurationFlags(item.Metric.Duration, item.Metric, stats)
}

// flagEmojis concatenates emoji for a set of flags.
func flagEmojis(flags []string) string {
	return format.FlagEmojis(flags)
}

// --- Helpers ---
