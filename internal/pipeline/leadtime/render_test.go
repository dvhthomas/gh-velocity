package leadtime

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func makeBulkItems(n int) []BulkItem {
	items := make([]BulkItem, n)
	now := time.Now()
	for i := range n {
		d := time.Duration(i+1) * time.Hour
		closed := now.Add(-time.Duration(i) * 24 * time.Hour)
		items[i] = BulkItem{
			Issue: model.Issue{
				Number:   i + 1,
				Title:    fmt.Sprintf("Issue %d", i+1),
				URL:      fmt.Sprintf("https://github.com/org/repo/issues/%d", i+1),
				ClosedAt: &closed,
			},
			Metric: model.Metric{Duration: &d},
		}
	}
	return items
}

func TestWriteBulkMarkdown_Capped(t *testing.T) {
	items := makeBulkItems(60)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false}
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	err := WriteBulkMarkdown(rc, "org/repo", since, until, items, model.Stats{Count: 60}, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "showing 50 of 60") {
		t.Errorf("expected 'showing 50 of 60' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "10 more items not shown") {
		t.Errorf("expected '10 more items not shown' in output, got:\n%s", out)
	}
	// Count data rows by counting occurrences of "| #" (issue link) in table
	rows := strings.Count(out, "| #")
	if rows != 50 {
		t.Errorf("expected 50 table rows, got %d", rows)
	}
}

func TestWriteBulkMarkdown_NotCapped(t *testing.T) {
	items := makeBulkItems(10)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false}
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	err := WriteBulkMarkdown(rc, "org/repo", since, until, items, model.Stats{Count: 10}, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "showing") {
		t.Errorf("expected no 'showing' text when not capped, got:\n%s", out)
	}
	if strings.Contains(out, "more items not shown") {
		t.Errorf("expected no truncation message when not capped, got:\n%s", out)
	}
}

func TestWriteBulkPretty_Capped(t *testing.T) {
	items := makeBulkItems(60)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false, Width: 120}
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	err := WriteBulkPretty(rc, "org/repo", since, until, items, model.Stats{Count: 60}, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "10 more items not shown") {
		t.Errorf("expected '10 more items not shown' in output, got:\n%s", out)
	}
}

func TestWriteBulkPretty_NotCapped(t *testing.T) {
	items := makeBulkItems(10)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false, Width: 120}
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	err := WriteBulkPretty(rc, "org/repo", since, until, items, model.Stats{Count: 10}, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "more items not shown") {
		t.Errorf("expected no truncation message when not capped, got:\n%s", out)
	}
}
