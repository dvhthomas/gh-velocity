package quality

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func makeDetail(nBugs, nOther int) Detail {
	items := make([]QualityItem, 0, nBugs+nOther)
	for i := range nBugs {
		d := time.Duration(i+1) * time.Hour
		items = append(items, QualityItem{
			Number:      i + 1,
			Title:       fmt.Sprintf("Bug %d", i+1),
			URL:         fmt.Sprintf("https://github.com/org/repo/issues/%d", i+1),
			Category:    "bug",
			LeadTime:    format.FormatDuration(d),
			LeadTimeDur: &d,
		})
	}
	for i := range nOther {
		n := nBugs + i + 1
		d := time.Duration(n) * time.Hour
		items = append(items, QualityItem{
			Number:      n,
			Title:       fmt.Sprintf("Feature %d", n),
			URL:         fmt.Sprintf("https://github.com/org/repo/issues/%d", n),
			Category:    "feature",
			LeadTime:    format.FormatDuration(d),
			LeadTimeDur: &d,
		})
	}
	return Detail{
		Repository: "org/repo",
		Since:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until:      time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		Quality: model.StatsQuality{
			BugCount:    nBugs,
			TotalIssues: nBugs + nOther,
			BugRatio:    float64(nBugs) / float64(nBugs+nOther),
		},
		Items:      items,
		Categories: BuildCategories(items),
	}
}

func TestWriteMarkdown_BugsOnlyFilter(t *testing.T) {
	d := makeDetail(10, 20)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false}

	if err := WriteMarkdown(rc, d); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "Bug Details") {
		t.Error("expected 'Bug Details' heading")
	}
	// The detail table should only contain bug rows, not feature rows.
	// Count rows in the detail section (after <details> tag).
	detailIdx := strings.Index(out, "<details>")
	if detailIdx < 0 {
		t.Fatal("expected <details> section")
	}
	detailSection := out[detailIdx:]
	if strings.Contains(detailSection, "| feature |") {
		t.Error("expected no 'feature' items in detail table")
	}
	rows := strings.Count(detailSection, "| #")
	if rows != 10 {
		t.Errorf("expected 10 bug rows, got %d", rows)
	}
}

func TestWriteMarkdown_BugsCapped(t *testing.T) {
	d := makeDetail(60, 20)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false}

	if err := WriteMarkdown(rc, d); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "showing 50 of 60") {
		t.Errorf("expected 'showing 50 of 60' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "10 more bugs not shown") {
		t.Errorf("expected '10 more bugs not shown' in output")
	}
	rows := strings.Count(out, "| #")
	if rows != 50 {
		t.Errorf("expected 50 table rows, got %d", rows)
	}
}

func TestWriteMarkdown_NoBugs(t *testing.T) {
	d := makeDetail(0, 10)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false}

	if err := WriteMarkdown(rc, d); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "<details>") {
		t.Error("expected no details section when there are no bugs")
	}
}

func TestWritePretty_BugsOnlyFilter(t *testing.T) {
	d := makeDetail(10, 20)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false, Width: 120}

	if err := WritePretty(rc, d); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "Feature") {
		t.Error("expected no feature items in pretty output")
	}
}

func TestWritePretty_BugsCapped(t *testing.T) {
	d := makeDetail(60, 20)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false, Width: 120}

	if err := WritePretty(rc, d); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "10 more bugs not shown") {
		t.Errorf("expected '10 more bugs not shown' in output, got:\n%s", out)
	}
}

func TestWritePretty_NotCapped(t *testing.T) {
	d := makeDetail(10, 20)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false, Width: 120}

	if err := WritePretty(rc, d); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "more bugs not shown") {
		t.Error("expected no truncation message when not capped")
	}
}
