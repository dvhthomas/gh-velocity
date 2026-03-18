package throughput

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"text/template"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

//go:embed templates/*.md.tmpl
var templateFS embed.FS

var markdownTmpl = template.Must(
	template.New("throughput.md.tmpl").Funcs(format.TemplateFuncMap()).ParseFS(templateFS, "templates/throughput.md.tmpl"),
)

// --- JSON ---

type jsonThroughputInsight struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type jsonOutput struct {
	Repository string                  `json:"repository"`
	Window     format.JSONWindow       `json:"window"`
	SearchURL  string                  `json:"search_url"`
	Insights   []jsonThroughputInsight `json:"insights,omitempty"`
	Issues     int                     `json:"issues_closed"`
	PRs        int                     `json:"prs_merged"`
	Total      int                     `json:"total"`
	Warnings   []string                `json:"warnings,omitempty"`
}

// WriteJSON writes throughput as JSON.
func WriteJSON(w io.Writer, r model.ThroughputResult, searchURL string, warnings []string, insights []model.Insight) error {
	var jsonIns []jsonThroughputInsight
	for _, ins := range insights {
		jsonIns = append(jsonIns, jsonThroughputInsight{Type: ins.Type, Message: ins.Message})
	}
	out := jsonOutput{
		Repository: r.Repository,
		Window: format.JSONWindow{
			Since: r.Since.UTC().Format(time.RFC3339),
			Until: r.Until.UTC().Format(time.RFC3339),
		},
		SearchURL: searchURL,
		Insights:  jsonIns,
		Issues:    r.IssuesClosed,
		PRs:       r.PRsMerged,
		Total:     r.IssuesClosed + r.PRsMerged,
		Warnings:  warnings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Markdown ---

// CategoryRow holds a single row of the category breakdown table.
type CategoryRow struct {
	Name  string
	Count int
	Pct   int
}

type templateData struct {
	Repository string
	Since      time.Time
	Until      time.Time
	Insights   []string
	Issues     int
	PRs        int
	Total      int
	SearchURL  string
	Categories []CategoryRow
}

// WriteMarkdown writes throughput as markdown.
func WriteMarkdown(w io.Writer, r model.ThroughputResult, searchURL string, insights []model.Insight) error {
	return WriteMarkdownWithCategories(w, r, searchURL, insights, nil)
}

// WriteMarkdownWithCategories writes throughput as markdown with an optional category breakdown.
func WriteMarkdownWithCategories(w io.Writer, r model.ThroughputResult, searchURL string, insights []model.Insight, categories []CategoryRow) error {
	var insightMsgs []string
	for _, ins := range insights {
		insightMsgs = append(insightMsgs, format.LinkStatTerms(ins.Message))
	}
	return markdownTmpl.Execute(w, templateData{
		Repository: r.Repository,
		Since:      r.Since,
		Until:      r.Until,
		Insights:   insightMsgs,
		Issues:     r.IssuesClosed,
		PRs:        r.PRsMerged,
		Total:      r.IssuesClosed + r.PRsMerged,
		SearchURL:  searchURL,
		Categories: categories,
	})
}

// --- Pretty ---

// WritePretty writes throughput as formatted text.
func WritePretty(w io.Writer, r model.ThroughputResult, searchURL string, insights []model.Insight) error {
	fmt.Fprintf(w, "Throughput: %s (%s – %s UTC)\n\n",
		r.Repository, r.Since.UTC().Format(time.DateOnly), r.Until.UTC().Format(time.DateOnly))
	model.WriteInsightsPretty(w, insights)
	fmt.Fprintf(w, "  Issues closed: %d\n", r.IssuesClosed)
	fmt.Fprintf(w, "  PRs merged:    %d\n", r.PRsMerged)
	fmt.Fprintf(w, "  Total:         %d\n", r.IssuesClosed+r.PRsMerged)
	if r.IssuesClosed+r.PRsMerged == 0 && searchURL != "" {
		fmt.Fprintf(w, "\n  No activity in this period.\n")
		fmt.Fprintf(w, "  Verify: %s\n", searchURL)
	}
	return nil
}
