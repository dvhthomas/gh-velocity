package format

import (
	"fmt"
	"io"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// WriteCycleTimeMarkdown writes a single cycle-time result as a markdown table.
func WriteCycleTimeMarkdown(w io.Writer, kind string, number int, title string, ct model.Metric) error {
	fmt.Fprintf(w, "| %s | Title | Started (UTC) | Cycle Time |\n", kind)
	fmt.Fprintf(w, "| ---: | --- | --- | --- |\n")
	startedStr := "N/A"
	if ct.Start != nil {
		startedStr = ct.Start.Time.UTC().Format(time.DateOnly)
	}
	fmt.Fprintf(w, "| #%d | %s | %s | %s |\n", number, sanitizeMarkdown(title), startedStr, FormatMetric(ct))
	return nil
}

// WriteCycleTimePretty writes a single cycle-time result as formatted text.
func WriteCycleTimePretty(w io.Writer, kind string, number int, title, strategy string, ct model.Metric) error {
	fmt.Fprintf(w, "%s #%d  %s\n", kind, number, title)
	fmt.Fprintf(w, "  Strategy:   %s\n", strategy)
	if ct.Start != nil {
		fmt.Fprintf(w, "  Started:    %s UTC\n", ct.Start.Time.UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(w, "  Cycle Time: %s\n", FormatMetric(ct))
	return nil
}
