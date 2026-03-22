package cycletime

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

var (
	singleMarkdownTmpl = template.Must(
		template.New("cycletime.md.tmpl").Funcs(format.TemplateFuncMap()).ParseFS(templateFS, "templates/cycletime.md.tmpl"),
	)
	bulkMarkdownTmpl = template.Must(
		template.New("cycletime-bulk.md.tmpl").Funcs(format.TemplateFuncMap()).ParseFS(templateFS, "templates/cycletime-bulk.md.tmpl"),
	)
)

// ============================================================
// Single Issue JSON
// ============================================================

type jsonSingleOutput struct {
	Repository string            `json:"repository"`
	Issue      int               `json:"issue,omitempty"`
	PR         int               `json:"pr,omitempty"`
	Title      string            `json:"title"`
	State      string            `json:"state"`
	URL        string            `json:"url,omitempty"`
	Labels     []string          `json:"labels,omitempty"`
	CycleTime  format.JSONMetric `json:"cycle_time"`
	Warnings   []string          `json:"warnings,omitempty"`
}

// WriteIssueJSON writes cycle-time metrics for an issue as JSON.
func WriteIssueJSON(w io.Writer, repo string, issueNumber int, title, state, itemURL string, labels []string, m model.Metric, warnings []string) error {
	out := jsonSingleOutput{
		Repository: repo,
		Issue:      issueNumber,
		Title:      title,
		State:      state,
		URL:        itemURL,
		Labels:     labels,
		CycleTime:  format.MetricToJSON(m),
		Warnings:   warnings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WritePRJSON writes cycle-time metrics for a PR as JSON.
func WritePRJSON(w io.Writer, repo string, prNumber int, title, state, itemURL string, labels []string, m model.Metric, warnings []string) error {
	out := jsonSingleOutput{
		Repository: repo,
		PR:         prNumber,
		Title:      title,
		State:      state,
		URL:        itemURL,
		Labels:     labels,
		CycleTime:  format.MetricToJSON(m),
		Warnings:   warnings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ============================================================
// Single Markdown
// ============================================================

type singleTemplateData struct {
	Kind      string
	Link      string
	Title     string
	Started   string
	CycleTime string
}

// WriteMarkdown writes a single cycle-time result as a markdown table.
func WriteMarkdown(rc format.RenderContext, kind string, number int, title, itemURL string, ct model.Metric) error {
	startedStr := "N/A"
	if ct.Start != nil {
		startedStr = ct.Start.Time.UTC().Format(time.DateOnly)
	}
	return singleMarkdownTmpl.Execute(rc.Writer, singleTemplateData{
		Kind:      kind,
		Link:      format.FormatItemLink(number, itemURL, rc),
		Title:     format.SanitizeMarkdown(title),
		Started:   startedStr,
		CycleTime: format.FormatMetric(ct),
	})
}

// ============================================================
// Single Pretty
// ============================================================

// WritePretty writes a single cycle-time result as formatted text.
func WritePretty(rc format.RenderContext, kind string, number int, title, itemURL, strategy string, ct model.Metric) error {
	fmt.Fprintf(rc.Writer, "%s %s  %s\n", kind, format.FormatItemLink(number, itemURL, rc), title)
	fmt.Fprintf(rc.Writer, "  Strategy:   %s\n", strategy)
	if ct.Start != nil {
		fmt.Fprintf(rc.Writer, "  Started:    %s UTC\n", ct.Start.Time.UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(rc.Writer, "  Cycle Time: %s\n", format.FormatMetric(ct))
	return nil
}

// ============================================================
// Bulk JSON
// ============================================================

type jsonBulkOutput struct {
	Repository string               `json:"repository"`
	Window     format.JSONWindow    `json:"window"`
	SearchURL  string               `json:"search_url"`
	Strategy   string               `json:"strategy"`
	Sort       format.JSONSort      `json:"sort"`
	Insights   []format.JSONInsight `json:"insights,omitempty"`
	Items      []jsonBulkItem       `json:"items"`
	Stats      format.JSONStats     `json:"stats"`
	Capped     bool                 `json:"capped,omitempty"`
	Warnings   []string             `json:"warnings,omitempty"`
}

type jsonBulkItem struct {
	Number    int               `json:"number"`
	Title     string            `json:"title"`
	URL       string            `json:"url,omitempty"`
	Labels    []string          `json:"labels,omitempty"`
	CycleTime format.JSONMetric `json:"cycle_time"`
	Flags     []string          `json:"flags,omitempty"`
}

// WriteBulkJSON writes bulk cycle-time results as JSON.
func WriteBulkJSON(w io.Writer, repo string, since, until time.Time, strategy string, items []BulkItem, stats model.Stats, searchURL string, warnings []string, insights []model.Insight) error {
	sorted := format.SortBy(items, "cycle_time", format.Desc, func(it BulkItem) *time.Duration { return it.Metric.Duration })
	jsonIns := format.InsightsToJSON(insights)
	out := jsonBulkOutput{
		Repository: repo,
		Window: format.JSONWindow{
			Since: since.UTC().Format(time.RFC3339),
			Until: until.UTC().Format(time.RFC3339),
		},
		SearchURL: searchURL,
		Strategy:  strategy,
		Sort:      sorted.JSONSort(),
		Insights:  jsonIns,
		Items:     make([]jsonBulkItem, 0, len(sorted.Items)),
		Stats:     format.StatsToJSON(stats),
		Capped:    len(items) >= 1000,
		Warnings:  warnings,
	}

	for _, item := range sorted.Items {
		out.Items = append(out.Items, jsonBulkItem{
			Number:    item.Issue.Number,
			Title:     item.Issue.Title,
			URL:       item.Issue.URL,
			Labels:    item.Issue.Labels,
			CycleTime: format.MetricToJSON(item.Metric),
			Flags:     classifyFlags(item, stats),
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
	Repository  string
	Since       time.Time
	Until       time.Time
	Strategy    string
	Insights    []string
	Items       []bulkItemRow
	Detail      []string
	Summary     string
	SearchURL   string
	SortHeader  string
	DetailCount int  // items with cycle time data (before capping)
	TotalCount  int  // total items including those without data
	Capped      bool // true when items were truncated to MaxDetailRows
}

type bulkItemRow struct {
	Link      string
	Title     string
	Closed    string
	CycleTime string
	Flag      string
}

// WriteBulkMarkdown writes bulk cycle-time results as markdown.
// Items without cycle time data (Duration == nil) are filtered from the detail
// table to avoid cluttering it with N/A rows.
func WriteBulkMarkdown(rc format.RenderContext, repo string, since, until time.Time, strategy string, items []BulkItem, stats model.Stats, searchURL string, insights []model.Insight) error {
	sorted := format.SortBy(items, "cycle_time", format.Desc, func(it BulkItem) *time.Duration { return it.Metric.Duration })
	var insightMsgs []string
	for _, ins := range insights {
		insightMsgs = append(insightMsgs, format.LinkStatTerms(ins.Message))
	}

	// Build filtered rows (exclude N/A items).
	var rows []bulkItemRow
	for _, item := range sorted.Items {
		if item.Metric.Duration == nil {
			continue
		}
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		rows = append(rows, bulkItemRow{
			Link:      format.FormatItemLink(item.Issue.Number, item.Issue.URL, rc),
			Title:     format.SanitizeMarkdown(item.Issue.Title),
			Closed:    closedStr,
			CycleTime: format.FormatMetricDuration(item.Metric),
			Flag:      flagEmojis(classifyFlags(item, stats)),
		})
	}

	detailCount := len(rows)
	capped := detailCount > format.MaxDetailRows
	if capped {
		rows = rows[:format.MaxDetailRows]
	}

	data := bulkTemplateData{
		Repository:  repo,
		Since:       since,
		Until:       until,
		Strategy:    strategy,
		Insights:    insightMsgs,
		Detail:      format.FormatStatsDetail(stats),
		Summary:     format.FormatStatsSummary(stats),
		SearchURL:   searchURL,
		SortHeader:  sorted.Header("cycle_time", "Cycle Time"),
		TotalCount:  len(sorted.Items),
		DetailCount: detailCount,
		Capped:      capped,
		Items:       rows,
	}
	return bulkMarkdownTmpl.Execute(rc.Writer, data)
}

// ============================================================
// Bulk Pretty
// ============================================================

// WriteBulkPretty writes bulk cycle-time results as a formatted table.
func WriteBulkPretty(rc format.RenderContext, repo string, since, until time.Time, strategy string, items []BulkItem, stats model.Stats, searchURL string, insights []model.Insight) error {
	sorted := format.SortBy(items, "cycle_time", format.Desc, func(it BulkItem) *time.Duration { return it.Metric.Duration })

	fmt.Fprintf(rc.Writer, "Cycle Time: %s (%s – %s UTC) [%s]\n\n",
		repo, since.UTC().Format(time.DateOnly), until.UTC().Format(time.DateOnly), strategy)
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

	// Filter out items without cycle time data, then cap.
	var filtered []BulkItem
	for _, item := range sorted.Items {
		if item.Metric.Duration == nil {
			continue
		}
		filtered = append(filtered, item)
	}

	total := len(filtered)
	capped := total > format.MaxDetailRows
	if capped {
		filtered = filtered[:format.MaxDetailRows]
	}

	tp := format.NewTable(rc.Writer, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"", "#", "Title", "Closed", sorted.Header("cycle_time", "Cycle Time")})
	for _, item := range filtered {
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

func flagEmojis(flags []string) string {
	return format.FlagEmojis(flags)
}

// --- Helpers ---
