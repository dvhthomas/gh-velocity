package format

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

type jsonThroughputOutput struct {
	Repository string     `json:"repository"`
	Window     jsonWindow `json:"window"`
	SearchURL  string     `json:"search_url"`
	Issues     int        `json:"issues_closed"`
	PRs        int        `json:"prs_merged"`
	Total      int        `json:"total"`
}

// WriteThroughputJSON writes throughput as JSON.
func WriteThroughputJSON(w io.Writer, r model.ThroughputResult, searchURL string) error {
	out := jsonThroughputOutput{
		Repository: r.Repository,
		Window: jsonWindow{
			Since: r.Since.UTC().Format(time.RFC3339),
			Until: r.Until.UTC().Format(time.RFC3339),
		},
		SearchURL: searchURL,
		Issues:    r.IssuesClosed,
		PRs:       r.PRsMerged,
		Total:     r.IssuesClosed + r.PRsMerged,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteThroughputMarkdown writes throughput as markdown using an embedded template.
func WriteThroughputMarkdown(w io.Writer, r model.ThroughputResult, searchURL string) error {
	return renderThroughputMarkdown(w, r, searchURL)
}

// WriteThroughputPretty writes throughput as formatted text.
func WriteThroughputPretty(w io.Writer, r model.ThroughputResult, searchURL string) error {
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
