package format

import (
	"embed"
	"fmt"
	"io"
	"maps"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

//go:embed templates/*.md.tmpl
var templateFS embed.FS

// funcMap provides shared template functions for markdown formatters.
var funcMap = template.FuncMap{
	"date": func(t time.Time) string {
		return t.UTC().Format(time.DateOnly)
	},
	"rfc3339": func(t time.Time) string {
		return t.UTC().Format(time.RFC3339)
	},
	"datePtr": func(t *time.Time) string {
		if t == nil {
			return "N/A"
		}
		return t.UTC().Format(time.DateOnly)
	},
	"duration":       FormatDuration,
	"durationPtr":    FormatDurationPtr,
	"metricDuration": FormatMetricDuration,
	"metric":         FormatMetric,
	"sanitize":       SanitizeMarkdown,
	"labels":         FormatLabels,
	"itemLink": func(number int, url string) string {
		if url == "" {
			return fmt.Sprintf("#%d", number)
		}
		return fmt.Sprintf("[#%d](%s)", number, stripControlChars(url))
	},
	"releaseLink": func(name, url string) string {
		if url == "" {
			return SanitizeMarkdown(name)
		}
		return fmt.Sprintf("[%s](%s)", SanitizeMarkdown(name), stripControlChars(url))
	},
	"pct": func(f float64) string {
		return fmt.Sprintf("%.0f%%", f*100)
	},
	"join": strings.Join,
	"docLink": func(text, anchor string) string {
		return DocLink(text, anchor)
	},
}

// mustParseTemplate parses a named template from the embedded filesystem.
func mustParseTemplate(name string) *template.Template {
	return template.Must(
		template.New(name).Funcs(funcMap).ParseFS(templateFS, "templates/"+name),
	)
}

// TemplateFuncMap returns a copy of the shared template function map.
// Per-metric packages extend this with metric-specific functions.
func TemplateFuncMap() template.FuncMap {
	fm := make(template.FuncMap, len(funcMap))
	maps.Copy(fm, funcMap)
	return fm
}

// ============================================================
// Provenance (shared "how to interpret" section)
// ============================================================

var provenanceMarkdownTmpl = mustParseTemplate("provenance.md.tmpl")

type provenanceRow struct {
	Key   string
	Value string
}

type provenanceTemplateData struct {
	Command    string
	Config     bool // whether to render the config table
	ConfigRows []provenanceRow
	Extra      string // additional markdown content from the caller
}

// RenderProvenanceMarkdown writes a <details> block with command, config,
// and optional extra markdown (command-specific content like effort strategy).
func RenderProvenanceMarkdown(w io.Writer, p model.Provenance, extra string) error {
	data := provenanceTemplateData{
		Command: p.Command,
		Config:  len(p.Config) > 0,
		Extra:   extra,
	}
	keys := sortedStringKeys(p.Config)
	for _, k := range keys {
		data.ConfigRows = append(data.ConfigRows, provenanceRow{Key: k, Value: p.Config[k]})
	}
	return provenanceMarkdownTmpl.Execute(w, data)
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ============================================================
// Report (dashboard summary)
// ============================================================

var reportMarkdownTmpl = mustParseTemplate("report.md.tmpl")

type reportTemplateData struct {
	Repository    string
	Since         time.Time
	Until         time.Time
	LeadTime      string
	CycleTime     string
	Throughput    string
	Velocity      string
	WIP           string
	Quality       string
	Warnings      []string
	HasInsights   bool
	HasData       bool // true when any section has data (for "not configured" rows)
	InsightGroups []insightGroup
}

// insightGroup represents a section's insights for the Key Findings block.
type insightGroup struct {
	Section  string
	Messages []string
}

func renderReportMarkdown(w io.Writer, r model.StatsResult) error {
	data := reportTemplateData{
		Repository: r.Repository,
		Since:      r.Since,
		Until:      r.Until,
	}
	if r.LeadTime != nil {
		data.LeadTime = FormatStatsSummary(*r.LeadTime)
	}
	if r.CycleTime != nil {
		data.CycleTime = FormatStatsSummary(*r.CycleTime)
	}
	if r.Throughput != nil {
		data.Throughput = fmt.Sprintf("%d issues closed, %d PRs merged",
			r.Throughput.IssuesClosed, r.Throughput.PRsMerged)
	}
	if r.Velocity != nil {
		data.Velocity = FormatVelocitySummary(*r.Velocity)
	}
	if r.WIPCount != nil {
		data.WIP = fmt.Sprintf("%d items in progress", *r.WIPCount)
	}
	if r.Quality != nil {
		data.Quality = fmt.Sprintf("%d bugs / %d issues (%.0f%% defect rate)",
			r.Quality.BugCount, r.Quality.TotalIssues, r.Quality.DefectRate*100)
	}
	data.Warnings = r.Warnings
	data.InsightGroups = buildInsightGroups(r)
	// Apply doc links to stat terms in markdown output.
	for i := range data.InsightGroups {
		for j := range data.InsightGroups[i].Messages {
			data.InsightGroups[i].Messages[j] = LinkStatTerms(data.InsightGroups[i].Messages[j])
		}
	}
	data.HasInsights = len(data.InsightGroups) > 0
	data.HasData = data.LeadTime != "" || data.CycleTime != "" || data.Throughput != "" || data.Velocity != "" || data.Quality != ""
	return reportMarkdownTmpl.Execute(w, data)
}

// ============================================================
// WIP
// ============================================================

var wipMarkdownTmpl = mustParseTemplate("wip.md.tmpl")

type wipTemplateData struct {
	Repository string
	Items      []wipItemRow
	Count      int
}

type wipItemRow struct {
	Link         string
	Title        string
	Labels       string
	Status       string
	Age          string
	Kind         string
	LastActivity string
	Staleness    string
}

func renderWIPMarkdown(w io.Writer, rc RenderContext, repo string, items []model.WIPItem) error {
	sorted := sortWIPByAgeDesc(items)
	data := wipTemplateData{
		Repository: repo,
		Count:      len(items),
	}
	for _, item := range sorted {
		link := ""
		if item.Number > 0 {
			link = FormatItemLink(item.Number, item.URL, rc)
		}
		data.Items = append(data.Items, wipItemRow{
			Link:         link,
			Title:        SanitizeMarkdown(item.Title),
			Labels:       FormatLabels(item.Labels),
			Status:       item.Status,
			Age:          FormatDuration(item.Age),
			Kind:         item.Kind,
			LastActivity: formatLastActivity(item.UpdatedAt),
			Staleness:    string(item.Staleness),
		})
	}
	return wipMarkdownTmpl.Execute(w, data)
}

// ============================================================
// Scope
// ============================================================

var scopeMarkdownTmpl = mustParseTemplate("scope.md.tmpl")

type scopeTemplateData struct {
	Tag         string
	PreviousTag string
	Strategies  []scopeStrategyRow
	MergedCount int
	IssueCount  int
	PRCount     int
}

type scopeStrategyRow struct {
	Name  string
	Count int
	Items []scopeItemRow
}

type scopeItemRow struct {
	Number int
	Title  string
	PRRef  string
}

func renderScopeMarkdown(w io.Writer, result *model.ScopeResult) error {
	issueCount, prCount := countMergedTypes(result.Merged)
	data := scopeTemplateData{
		Tag:         result.Tag,
		PreviousTag: result.PreviousTag,
		MergedCount: len(result.Merged),
		IssueCount:  issueCount,
		PRCount:     prCount,
	}
	for _, sr := range result.Strategies {
		row := scopeStrategyRow{Name: sr.Name, Count: len(sr.Items)}
		for _, item := range sr.Items {
			num, title, prRef := 0, "", ""
			if item.Issue != nil {
				num = item.Issue.Number
				title = item.Issue.Title
			}
			if item.PR != nil {
				if item.Issue == nil {
					num = item.PR.Number
					title = item.PR.Title
				}
				prRef = fmt.Sprintf("#%d", item.PR.Number)
			}
			row.Items = append(row.Items, scopeItemRow{Number: num, Title: title, PRRef: prRef})
		}
		data.Strategies = append(data.Strategies, row)
	}
	return scopeMarkdownTmpl.Execute(w, data)
}

// ============================================================
// My Week
// ============================================================

var myweekMarkdownTmpl = mustParseTemplate("myweek.md.tmpl")

type myweekTemplateData struct {
	Login    string
	Repo     string
	Since    time.Time
	Until    time.Time
	Insights []string
	// Lookback
	IssuesClosed    []myweekItemRow
	PRsMerged       []myweekItemRow
	PRsReviewed     []myweekItemRow
	Releases        []myweekReleaseRow
	IssuesClosedURL string
	PRsMergedURL    string
	PRsReviewedURL  string
	// Lookahead
	IssuesOpen []myweekAnnotatedRow
	PRsOpen    []myweekAnnotatedRow
	// Review queue
	ReviewQueue []myweekReviewRow
}

type myweekItemRow struct {
	Link  string
	Title string
	Date  string
}

type myweekReleaseRow struct {
	Link string
	Date string
}

type myweekAnnotatedRow struct {
	Link   string
	Title  string
	Status string
}

type myweekReviewRow struct {
	Link   string
	Title  string
	Author string
	Age    string
}

func renderMyWeekMarkdown(w io.Writer, rc RenderContext, r model.MyWeekResult, ins model.MyWeekInsights, urls MyWeekSearchURLs) error {
	data := myweekTemplateData{
		Login:           r.Login,
		Repo:            r.Repo,
		Since:           r.Since,
		Until:           r.Until,
		Insights:        buildInsightLines(r, ins),
		IssuesClosedURL: urls.IssuesClosed,
		PRsMergedURL:    urls.PRsMerged,
		PRsReviewedURL:  urls.PRsReviewed,
	}

	// Lookback: issues closed
	for _, iss := range r.IssuesClosed {
		dateStr := ""
		if iss.ClosedAt != nil {
			dateStr = " (" + iss.ClosedAt.Format(time.DateOnly) + ")"
		}
		data.IssuesClosed = append(data.IssuesClosed, myweekItemRow{
			Link:  FormatItemLink(iss.Number, iss.URL, rc),
			Title: SanitizeMarkdown(iss.Title),
			Date:  dateStr,
		})
	}

	// Lookback: PRs merged
	for _, pr := range r.PRsMerged {
		dateStr := ""
		if pr.MergedAt != nil {
			dateStr = " (" + pr.MergedAt.Format(time.DateOnly) + ")"
		}
		data.PRsMerged = append(data.PRsMerged, myweekItemRow{
			Link:  FormatItemLink(pr.Number, pr.URL, rc),
			Title: SanitizeMarkdown(pr.Title),
			Date:  dateStr,
		})
	}

	// Lookback: PRs reviewed
	for _, pr := range r.PRsReviewed {
		data.PRsReviewed = append(data.PRsReviewed, myweekItemRow{
			Link:  FormatItemLink(pr.Number, pr.URL, rc),
			Title: SanitizeMarkdown(pr.Title),
		})
	}

	// Lookback: releases
	for _, rel := range r.Releases {
		dateStr := rel.CreatedAt.Format(time.DateOnly)
		if rel.PublishedAt != nil {
			dateStr = rel.PublishedAt.Format(time.DateOnly)
		}
		name := rel.Name
		if name == "" {
			name = rel.TagName
		}
		link := SanitizeMarkdown(name)
		if rel.URL != "" {
			link = fmt.Sprintf("[%s](%s)", SanitizeMarkdown(name), rel.URL)
		}
		data.Releases = append(data.Releases, myweekReleaseRow{
			Link: link,
			Date: dateStr,
		})
	}

	// Lookahead: open issues
	for _, iss := range r.IssuesOpen {
		s := model.IssueStatus(iss, r.Since, r.Until)
		data.IssuesOpen = append(data.IssuesOpen, myweekAnnotatedRow{
			Link:   FormatItemLink(iss.Number, iss.URL, rc),
			Title:  SanitizeMarkdown(iss.Title),
			Status: formatStatusMarkdown(s),
		})
	}

	// Lookahead: open PRs
	for _, pr := range r.PRsOpen {
		nr := model.PRNeedsReview(pr, r.PRsNeedingReview)
		s := model.PRStatus(pr, nr, r.Since, r.Until)
		data.PRsOpen = append(data.PRsOpen, myweekAnnotatedRow{
			Link:   FormatItemLink(pr.Number, pr.URL, rc),
			Title:  SanitizeMarkdown(pr.Title),
			Status: formatStatusMarkdown(s),
		})
	}

	// Review queue
	for _, pr := range r.PRsAwaitingMyReview {
		age := model.DaysBetween(pr.CreatedAt, r.Until)
		author := pr.Author
		if author == "" {
			author = "unknown"
		}
		data.ReviewQueue = append(data.ReviewQueue, myweekReviewRow{
			Link:   FormatItemLink(pr.Number, pr.URL, rc),
			Title:  SanitizeMarkdown(pr.Title),
			Author: author,
			Age:    formatAge(age),
		})
	}

	return myweekMarkdownTmpl.Execute(w, data)
}
