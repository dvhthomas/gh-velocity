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

type jsonOutput struct {
	Repository string            `json:"repository"`
	Window     format.JSONWindow `json:"window"`
	SearchURL  string            `json:"search_url"`
	Issues     int               `json:"issues_closed"`
	PRs        int               `json:"prs_merged"`
	Total      int               `json:"total"`
	Warnings   []string          `json:"warnings,omitempty"`
}

// WriteJSON writes throughput as JSON.
func WriteJSON(w io.Writer, r model.ThroughputResult, searchURL string, warnings []string) error {
	out := jsonOutput{
		Repository: r.Repository,
		Window: format.JSONWindow{
			Since: r.Since.UTC().Format(time.RFC3339),
			Until: r.Until.UTC().Format(time.RFC3339),
		},
		SearchURL: searchURL,
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

type templateData struct {
	Repository string
	Since      time.Time
	Until      time.Time
	Issues     int
	PRs        int
	Total      int
	SearchURL  string
}

// WriteMarkdown writes throughput as markdown.
func WriteMarkdown(w io.Writer, r model.ThroughputResult, searchURL string) error {
	return markdownTmpl.Execute(w, templateData{
		Repository: r.Repository,
		Since:      r.Since,
		Until:      r.Until,
		Issues:     r.IssuesClosed,
		PRs:        r.PRsMerged,
		Total:      r.IssuesClosed + r.PRsMerged,
		SearchURL:  searchURL,
	})
}

// --- Pretty ---

// WritePretty writes throughput as formatted text.
func WritePretty(w io.Writer, r model.ThroughputResult, searchURL string) error {
	fmt.Fprintf(w, "Throughput: %s (%s – %s UTC)\n\n",
		r.Repository, r.Since.UTC().Format(time.DateOnly), r.Until.UTC().Format(time.DateOnly))
	fmt.Fprintf(w, "  Issues closed: %d\n", r.IssuesClosed)
	fmt.Fprintf(w, "  PRs merged:    %d\n", r.PRsMerged)
	fmt.Fprintf(w, "  Total:         %d\n", r.IssuesClosed+r.PRsMerged)
	if r.IssuesClosed+r.PRsMerged == 0 && searchURL != "" {
		fmt.Fprintf(w, "\n  No activity in this period.\n")
		fmt.Fprintf(w, "  Verify: %s\n", searchURL)
	}
	return nil
}
