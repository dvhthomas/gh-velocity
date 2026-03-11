package format

import (
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// WriteCycleTimeMarkdown writes a single cycle-time result as a markdown table.
func WriteCycleTimeMarkdown(rc RenderContext, kind string, number int, title, itemURL string, ct model.Metric) error {
	fmt.Fprintf(rc.Writer, "| %s | Title | Started (UTC) | Cycle Time |\n", kind)
	fmt.Fprintf(rc.Writer, "| ---: | --- | --- | --- |\n")
	startedStr := "N/A"
	if ct.Start != nil {
		startedStr = ct.Start.Time.UTC().Format(time.DateOnly)
	}
	fmt.Fprintf(rc.Writer, "| %s | %s | %s | %s |\n", FormatItemLink(number, itemURL, rc), sanitizeMarkdown(title), startedStr, FormatMetric(ct))
	return nil
}

// WriteCycleTimePretty writes a single cycle-time result as formatted text.
func WriteCycleTimePretty(rc RenderContext, kind string, number int, title, itemURL, strategy string, ct model.Metric) error {
	fmt.Fprintf(rc.Writer, "%s %s  %s\n", kind, FormatItemLink(number, itemURL, rc), title)
	fmt.Fprintf(rc.Writer, "  Strategy:   %s\n", strategy)
	if ct.Start != nil {
		fmt.Fprintf(rc.Writer, "  Started:    %s UTC\n", ct.Start.Time.UTC().Format(time.RFC3339))
	}
	fmt.Fprintf(rc.Writer, "  Cycle Time: %s\n", FormatMetric(ct))
	return nil
}
