package format

import (
	"embed"
	"fmt"
	"io"
	"math"
	"text/template"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/metrics"
)

//go:embed templates/*.md.tmpl
var templateFS embed.FS

// funcMap provides shared template functions for markdown formatters.
var funcMap = template.FuncMap{
	"date": func(t time.Time) string {
		return t.UTC().Format(time.DateOnly)
	},
	"primary": func(p metrics.PathRisk) string {
		if p.ContributorCount >= 3 && p.PrimaryPct <= 70 {
			return "distributed"
		}
		return fmt.Sprintf("%s (%.0f%%)", p.Primary.Name, p.PrimaryPct)
	},
}

// mustParseTemplate parses a named template from the embedded filesystem.
func mustParseTemplate(name string) *template.Template {
	return template.Must(
		template.New(name).Funcs(funcMap).ParseFS(templateFS, "templates/"+name),
	)
}

// busFactorMarkdownTmpl is the parsed bus-factor markdown template.
var busFactorMarkdownTmpl = mustParseTemplate("busfactor.md.tmpl")

// busFactorTemplateData prepares template data from a BusFactorResult.
type busFactorTemplateData struct {
	Repository string
	Since      time.Time
	Depth      int
	MinCommits int
	Days       int
	Paths      []metrics.PathRisk
	SummaryLine string
}

// renderBusFactorMarkdown executes the bus-factor markdown template.
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
