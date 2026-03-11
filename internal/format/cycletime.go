package format

import (
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// WriteCycleTimeMarkdown writes a single cycle-time result as a markdown table using an embedded template.
func WriteCycleTimeMarkdown(rc RenderContext, kind string, number int, title, itemURL string, ct model.Metric) error {
	return renderCycleTimeMarkdown(rc.Writer, rc, kind, number, title, itemURL, ct)
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
