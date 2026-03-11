package format

import (
	"embed"
	"fmt"
	"io"
	"math"
	"strings"
	"text/template"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
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
	"duration": FormatDuration,
	"durationPtr": FormatDurationPtr,
	"metricDuration": FormatMetricDuration,
	"metric": FormatMetric,
	"sanitize": sanitizeMarkdown,
	"labels":   FormatLabels,
	"primary": func(p metrics.PathRisk) string {
		if p.ContributorCount >= 3 && p.PrimaryPct <= 70 {
			return "distributed"
		}
		return fmt.Sprintf("%s (%.0f%%)", p.Primary.Name, p.PrimaryPct)
	},
	"itemLink": func(number int, url string) string {
		if url == "" {
			return fmt.Sprintf("#%d", number)
		}
		return fmt.Sprintf("[#%d](%s)", number, stripControlChars(url))
	},
	"releaseLink": func(name, url string) string {
		if url == "" {
			return sanitizeMarkdown(name)
		}
		return fmt.Sprintf("[%s](%s)", sanitizeMarkdown(name), stripControlChars(url))
	},
	"pct": func(f float64) string {
		return fmt.Sprintf("%.0f%%", f*100)
	},
	"join": strings.Join,
}

// mustParseTemplate parses a named template from the embedded filesystem.
func mustParseTemplate(name string) *template.Template {
	return template.Must(
		template.New(name).Funcs(funcMap).ParseFS(templateFS, "templates/"+name),
	)
}

// ============================================================
// Bus Factor
// ============================================================

var busFactorMarkdownTmpl = mustParseTemplate("busfactor.md.tmpl")

type busFactorTemplateData struct {
	Repository  string
	Since       time.Time
	Depth       int
	MinCommits  int
	Days        int
	Paths       []metrics.PathRisk
	SummaryLine string
}

func renderBusFactorMarkdown(w io.Writer, result metrics.BusFactorResult) error {
	days := int(math.Round(time.Since(result.Since).Hours() / 24))
	high, med, low := countRisks(result)

	data := busFactorTemplateData{
		Repository:  result.Repository,
		Since:       result.Since,
		Depth:       result.Depth,
		MinCommits:  result.MinCommits,
		Days:        days,
		Paths:       result.Paths,
		SummaryLine: fmt.Sprintf("%d HIGH risk, %d MEDIUM risk, %d LOW risk areas", high, med, low),
	}

	return busFactorMarkdownTmpl.Execute(w, data)
}

// ============================================================
// Throughput
// ============================================================

var throughputMarkdownTmpl = mustParseTemplate("throughput.md.tmpl")

type throughputTemplateData struct {
	Repository string
	Since      time.Time
	Until      time.Time
	Issues     int
	PRs        int
	Total      int
}

func renderThroughputMarkdown(w io.Writer, r model.ThroughputResult) error {
	return throughputMarkdownTmpl.Execute(w, throughputTemplateData{
		Repository: r.Repository,
		Since:      r.Since,
		Until:      r.Until,
		Issues:     r.IssuesClosed,
		PRs:        r.PRsMerged,
		Total:      r.IssuesClosed + r.PRsMerged,
	})
}

// ============================================================
// Report (dashboard summary)
// ============================================================

var reportMarkdownTmpl = mustParseTemplate("report.md.tmpl")

type reportTemplateData struct {
	Repository string
	Since      time.Time
	Until      time.Time
	LeadTime   string
	CycleTime  string
	Throughput string
	WIP        string
	Quality    string
	Warnings   []string
}

func renderReportMarkdown(w io.Writer, r model.StatsResult) error {
	data := reportTemplateData{
		Repository: r.Repository,
		Since:      r.Since,
		Until:      r.Until,
	}
	if r.LeadTime != nil {
		data.LeadTime = formatStatsSummary(*r.LeadTime)
	}
	if r.CycleTime != nil {
		data.CycleTime = formatStatsSummary(*r.CycleTime)
	}
	if r.Throughput != nil {
		data.Throughput = fmt.Sprintf("%d issues closed, %d PRs merged",
			r.Throughput.IssuesClosed, r.Throughput.PRsMerged)
	}
	if r.WIPCount != nil {
		data.WIP = fmt.Sprintf("%d items in progress", *r.WIPCount)
	}
	if r.Quality != nil {
		data.Quality = fmt.Sprintf("%d bugs / %d issues (%.0f%% defect rate)",
			r.Quality.BugCount, r.Quality.TotalIssues, r.Quality.DefectRate*100)
	}
	data.Warnings = r.Warnings
	return reportMarkdownTmpl.Execute(w, data)
}

// ============================================================
// Release
// ============================================================

var releaseMarkdownTmpl = mustParseTemplate("release.md.tmpl")

type releaseTemplateData struct {
	Tag         string
	PreviousTag string
	Cadence     string
	IsHotfix    bool
	Categories  []releaseCategoryRow
	TotalIssues int
	Issues      []releaseIssueRow
	LeadTime    string
	CycleTime   string
	ReleaseLag  string
	Warnings    []string
}

type releaseCategoryRow struct {
	Name  string
	Count int
	Ratio string
}

type releaseIssueRow struct {
	Link      string
	Title     string
	Labels    string
	LeadTime  string
	CycleTime string
	RelLag    string
	Commits   int
	Flag      string
}

func renderReleaseMarkdown(w io.Writer, rc RenderContext, rm model.ReleaseMetrics, warnings []string) error {
	data := releaseTemplateData{
		Tag:         rm.Tag,
		PreviousTag: rm.PreviousTag,
		Cadence:     FormatDurationPtr(rm.Cadence),
		IsHotfix:    rm.IsHotfix,
		TotalIssues: rm.TotalIssues,
	}
	for _, name := range rm.CategoryNames {
		label := strings.ToUpper(name[:1]) + name[1:]
		data.Categories = append(data.Categories, releaseCategoryRow{
			Name:  label,
			Count: rm.CategoryCounts[name],
			Ratio: fmt.Sprintf("%.0f%%", rm.CategoryRatios[name]*100),
		})
	}
	for _, im := range rm.Issues {
		flag := ""
		if im.LeadTimeOutlier || im.CycleTimeOutlier {
			flag = "OUTLIER"
		}
		data.Issues = append(data.Issues, releaseIssueRow{
			Link:      FormatItemLink(im.Issue.Number, im.Issue.URL, rc),
			Title:     sanitizeMarkdown(im.Issue.Title),
			Labels:    FormatLabels(im.Issue.Labels),
			LeadTime:  FormatDurationPtr(im.LeadTime.Duration),
			CycleTime: FormatDurationPtr(im.CycleTime.Duration),
			RelLag:    FormatDurationPtr(im.ReleaseLag.Duration),
			Commits:   im.CommitCount,
			Flag:      flag,
		})
	}
	data.LeadTime = formatStatsRow(rm.LeadTimeStats)
	data.CycleTime = formatStatsRow(rm.CycleTimeStats)
	data.ReleaseLag = formatStatsRow(rm.ReleaseLagStats)
	data.Warnings = warnings
	return releaseMarkdownTmpl.Execute(w, data)
}

func formatStatsRow(s model.Stats) string {
	sd, p90, p95, outliers := "--", "--", "--", "--"
	if s.StdDev != nil {
		sd = FormatDuration(*s.StdDev)
	}
	if s.P90 != nil {
		p90 = FormatDuration(*s.P90)
	}
	if s.P95 != nil {
		p95 = FormatDuration(*s.P95)
	}
	if s.OutlierCutoff != nil {
		outliers = fmt.Sprintf("%d", s.OutlierCount)
	}
	return fmt.Sprintf("| %s | %s | %s | %s | %s | %s |",
		FormatDurationPtr(s.Mean), FormatDurationPtr(s.Median), sd, p90, p95, outliers)
}

// ============================================================
// Lead Time Bulk
// ============================================================

var leadtimeBulkMarkdownTmpl = mustParseTemplate("leadtime-bulk.md.tmpl")

type leadtimeBulkTemplateData struct {
	Repository string
	Since      time.Time
	Until      time.Time
	Items      []leadtimeItemRow
	Summary    string
}

type leadtimeItemRow struct {
	Link     string
	Title    string
	Labels   string
	Created  string
	Closed   string
	LeadTime string
}

func renderLeadTimeBulkMarkdown(w io.Writer, rc RenderContext, repo string, since, until time.Time, items []BulkLeadTimeItem, stats model.Stats) error {
	sorted := sortByCloseDateDesc(items)
	data := leadtimeBulkTemplateData{
		Repository: repo,
		Since:      since,
		Until:      until,
		Summary:    formatStatsSummary(stats),
	}
	for _, item := range sorted {
		closedStr := "N/A"
		if item.Issue.ClosedAt != nil {
			closedStr = item.Issue.ClosedAt.UTC().Format(time.DateOnly)
		}
		data.Items = append(data.Items, leadtimeItemRow{
			Link:     FormatItemLink(item.Issue.Number, item.Issue.URL, rc),
			Title:    sanitizeMarkdown(item.Issue.Title),
			Labels:   FormatLabels(item.Issue.Labels),
			Created:  item.Issue.CreatedAt.UTC().Format(time.DateOnly),
			Closed:   closedStr,
			LeadTime: FormatMetricDuration(item.Metric),
		})
	}
	return leadtimeBulkMarkdownTmpl.Execute(w, data)
}

// ============================================================
// Cycle Time Bulk
// ============================================================

var cycletimeBulkMarkdownTmpl = mustParseTemplate("cycletime-bulk.md.tmpl")

type cycletimeBulkTemplateData struct {
	Repository string
	Since      time.Time
	Until      time.Time
	Strategy   string
	Items      []cycletimeItemRow
	Summary    string
}

type cycletimeItemRow struct {
	Link      string
	Title     string
	Labels    string
	Started   string
	Closed    string
	CycleTime string
}

func renderCycleTimeBulkMarkdown(w io.Writer, rc RenderContext, repo string, since, until time.Time, strategy string, items []BulkCycleTimeItem, stats model.Stats) error {
	sorted := sortCycleByCloseDateDesc(items)
	data := cycletimeBulkTemplateData{
		Repository: repo,
		Since:      since,
		Until:      until,
		Strategy:   strategy,
		Summary:    formatStatsSummary(stats),
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
		data.Items = append(data.Items, cycletimeItemRow{
			Link:      FormatItemLink(item.Issue.Number, item.Issue.URL, rc),
			Title:     sanitizeMarkdown(item.Issue.Title),
			Labels:    FormatLabels(item.Issue.Labels),
			Started:   startedStr,
			Closed:    closedStr,
			CycleTime: FormatMetricDuration(item.Metric),
		})
	}
	return cycletimeBulkMarkdownTmpl.Execute(w, data)
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
			Title:        sanitizeMarkdown(item.Title),
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
// Single Cycle Time
// ============================================================

var cycletimeMarkdownTmpl = mustParseTemplate("cycletime.md.tmpl")

type cycletimeTemplateData struct {
	Kind      string
	Link      string
	Title     string
	Started   string
	CycleTime string
}

func renderCycleTimeMarkdown(w io.Writer, rc RenderContext, kind string, number int, title, itemURL string, ct model.Metric) error {
	startedStr := "N/A"
	if ct.Start != nil {
		startedStr = ct.Start.Time.UTC().Format(time.DateOnly)
	}
	return cycletimeMarkdownTmpl.Execute(w, cycletimeTemplateData{
		Kind:      kind,
		Link:      FormatItemLink(number, itemURL, rc),
		Title:     sanitizeMarkdown(title),
		Started:   startedStr,
		CycleTime: FormatMetric(ct),
	})
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
	IssuesClosed []myweekItemRow
	PRsMerged    []myweekItemRow
	PRsReviewed  []myweekItemRow
	Releases     []myweekReleaseRow
	// Lookahead
	IssuesOpen []myweekAnnotatedRow
	PRsOpen    []myweekAnnotatedRow
	// Review queue
	ReviewQueue []myweekReviewRow
}

type myweekItemRow struct {
	Link string
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

func renderMyWeekMarkdown(w io.Writer, rc RenderContext, r model.MyWeekResult) error {
	data := myweekTemplateData{
		Login:    r.Login,
		Repo:     r.Repo,
		Since:    r.Since,
		Until:    r.Until,
		Insights: buildInsightLines(r),
	}

	// Lookback: issues closed
	for _, iss := range r.IssuesClosed {
		dateStr := ""
		if iss.ClosedAt != nil {
			dateStr = " (" + iss.ClosedAt.Format(time.DateOnly) + ")"
		}
		data.IssuesClosed = append(data.IssuesClosed, myweekItemRow{
			Link:  FormatItemLink(iss.Number, iss.URL, rc),
			Title: sanitizeMarkdown(iss.Title),
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
			Title: sanitizeMarkdown(pr.Title),
			Date:  dateStr,
		})
	}

	// Lookback: PRs reviewed
	for _, pr := range r.PRsReviewed {
		data.PRsReviewed = append(data.PRsReviewed, myweekItemRow{
			Link:  FormatItemLink(pr.Number, pr.URL, rc),
			Title: sanitizeMarkdown(pr.Title),
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
		link := sanitizeMarkdown(name)
		if rel.URL != "" {
			link = fmt.Sprintf("[%s](%s)", sanitizeMarkdown(name), rel.URL)
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
			Title:  sanitizeMarkdown(iss.Title),
			Status: formatStatusMarkdown(s),
		})
	}

	// Lookahead: open PRs
	for _, pr := range r.PRsOpen {
		nr := model.PRNeedsReview(pr, r.PRsNeedingReview)
		s := model.PRStatus(pr, nr, r.Since, r.Until)
		data.PRsOpen = append(data.PRsOpen, myweekAnnotatedRow{
			Link:   FormatItemLink(pr.Number, pr.URL, rc),
			Title:  sanitizeMarkdown(pr.Title),
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
			Title:  sanitizeMarkdown(pr.Title),
			Author: author,
			Age:    formatAge(age),
		})
	}

	return myweekMarkdownTmpl.Execute(w, data)
}

// ============================================================
// Reviews (Review Pressure)
// ============================================================

var reviewsMarkdownTmpl = mustParseTemplate("reviews.md.tmpl")

type reviewsTemplateData struct {
	Repository string
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

func renderReviewsMarkdown(w io.Writer, rc RenderContext, result model.ReviewPressureResult) error {
	sorted := sortReviewsByAgeDesc(result.AwaitingReview)
	data := reviewsTemplateData{
		Repository: result.Repository,
		Count:      len(sorted),
	}
	for _, pr := range sorted {
		signal := ""
		if pr.IsStale {
			signal = "STALE"
			data.StaleCount++
		}
		data.Items = append(data.Items, reviewItemRow{
			Link:   FormatItemLink(pr.Number, pr.URL, rc),
			Title:  sanitizeMarkdown(pr.Title),
			Age:    FormatDuration(pr.Age),
			Signal: signal,
		})
	}
	return reviewsMarkdownTmpl.Execute(w, data)
}
