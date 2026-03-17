package cycletime

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

type jsonBulkInsight struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type jsonBulkOutput struct {
	Repository string            `json:"repository"`
	Window     format.JSONWindow `json:"window"`
	SearchURL  string            `json:"search_url"`
	Strategy   string            `json:"strategy"`
	Insights   []jsonBulkInsight `json:"insights,omitempty"`
	Items      []jsonBulkItem    `json:"items"`
	Stats      format.JSONStats  `json:"stats"`
	Capped     bool              `json:"capped,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
}

type jsonBulkItem struct {
	Number    int               `json:"number"`
	Title     string            `json:"title"`
	URL       string            `json:"url,omitempty"`
	Labels    []string          `json:"labels,omitempty"`
	CycleTime format.JSONMetric `json:"cycle_time"`
}

// WriteBulkJSON writes bulk cycle-time results as JSON.
func WriteBulkJSON(w io.Writer, repo string, since, until time.Time, strategy string, items []BulkItem, stats model.Stats, searchURL string, warnings []string, insights []model.Insight) error {
	var jsonIns []jsonBulkInsight
	for _, ins := range insights {
		jsonIns = append(jsonIns, jsonBulkInsight{Type: ins.Type, Message: ins.Message})
	}
	out := jsonBulkOutput{
		Repository: repo,
		Window: format.JSONWindow{
			Since: since.UTC().Format(time.RFC3339),
			Until: until.UTC().Format(time.RFC3339),
		},
		SearchURL: searchURL,
		Strategy:  strategy,
		Insights:  jsonIns,
		Items:     make([]jsonBulkItem, 0, len(items)),
		Stats:     format.StatsToJSON(stats),
		Capped:    len(items) >= 1000,
		Warnings:  warnings,
	}

	for _, item := range items {
		out.Items = append(out.Items, jsonBulkItem{
			Number:    item.Issue.Number,
			Title:     item.Issue.Title,
			URL:       item.Issue.URL,
			Labels:    item.Issue.Labels,
			CycleTime: format.MetricToJSON(item.Metric),
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
	Strategy   string
	Insights   []string
	Items      []bulkItemRow
	Summary    string
	SearchURL  string
}

type bulkItemRow struct {
	Link      string
	Title     string
	Labels    string
	Started   string
	Closed    string
	CycleTime string
}

// WriteBulkMarkdown writes bulk cycle-time results as markdown.
func WriteBulkMarkdown(rc format.RenderContext, repo string, since, until time.Time, strategy string, items []BulkItem, stats model.Stats, searchURL string, insights []model.Insight) error {
	sorted := sortByCloseDateDesc(items)
	var insightMsgs []string
	for _, ins := range insights {
		insightMsgs = append(insightMsgs, ins.Message)
	}
	data := bulkTemplateData{
		Repository: repo,
		Since:      since,
		Until:      until,
		Strategy:   strategy,
		Insights:   insightMsgs,
		Summary:    format.FormatStatsSummary(stats),
		SearchURL:  searchURL,
	}
	for _, item := range sorted {
		startedStr := "N/A"
		if item.Metric.Start != nil {
			startedStr = item.Metric.Start.Time.UTC().Format(time.DateOnly)
		}
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		data.Items = append(data.Items, bulkItemRow{
			Link:      format.FormatItemLink(item.Issue.Number, item.Issue.URL, rc),
			Title:     format.SanitizeMarkdown(item.Issue.Title),
			Labels:    format.FormatLabels(item.Issue.Labels),
			Started:   startedStr,
			Closed:    closedStr,
			CycleTime: format.FormatMetricDuration(item.Metric),
		})
	}
	return bulkMarkdownTmpl.Execute(rc.Writer, data)
}

// ============================================================
// Bulk Pretty
// ============================================================

// WriteBulkPretty writes bulk cycle-time results as a formatted table.
func WriteBulkPretty(rc format.RenderContext, repo string, since, until time.Time, strategy string, items []BulkItem, stats model.Stats, searchURL string, insights []model.Insight) error {
	sorted := sortByCloseDateDesc(items)

	fmt.Fprintf(rc.Writer, "Cycle Time: %s (%s – %s UTC) [%s]\n\n",
		repo, since.UTC().Format(time.DateOnly), until.UTC().Format(time.DateOnly), strategy)
	model.WriteInsightsPretty(rc.Writer, insights)

	if len(sorted) == 0 {
		fmt.Fprintln(rc.Writer, "  No issues closed in this period.")
		if searchURL != "" {
			fmt.Fprintf(rc.Writer, "  Verify: %s\n", searchURL)
		}
		return nil
	}

	tp := format.NewTable(rc.Writer, rc.IsTTY, rc.Width)
	tp.AddHeader([]string{"#", "Title", "Labels", "Started", "Closed", "Cycle Time"})
	for _, item := range sorted {
		startedStr := "N/A"
		if item.Metric.Start != nil {
			startedStr = item.Metric.Start.Time.UTC().Format(time.DateOnly)
		}
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		tp.AddField(format.FormatItemLink(item.Issue.Number, item.Issue.URL, rc))
		tp.AddField(item.Issue.Title)
		tp.AddField(format.FormatLabels(item.Issue.Labels))
		tp.AddField(startedStr)
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
