package cycletime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/metrics"
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

	err := WriteBulkMarkdown(rc, "org/repo", since, until, "issue", items, model.Stats{Count: 60}, "", nil)
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

	err := WriteBulkMarkdown(rc, "org/repo", since, until, "issue", items, model.Stats{Count: 10}, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "showing") {
		t.Errorf("expected no 'showing' text when not capped, got:\n%s", out)
	}
	if strings.Contains(out, "more items not shown") {
		t.Errorf("expected no truncation message when not capped")
	}
}

func TestWriteBulkPretty_Capped(t *testing.T) {
	items := makeBulkItems(60)
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false, Width: 120}
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	err := WriteBulkPretty(rc, "org/repo", since, until, "issue", items, model.Stats{Count: 60}, "", nil)
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

	err := WriteBulkPretty(rc, "org/repo", since, until, "issue", items, model.Stats{Count: 10}, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if strings.Contains(out, "more items not shown") {
		t.Errorf("expected no truncation message when not capped")
	}
}

// --- JSON render tests (regression safety for refactoring) ---

func TestWriteBulkJSON_Structure(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	d1 := 24 * time.Hour
	d2 := 48 * time.Hour
	closed1 := since.Add(72 * time.Hour)
	closed2 := since.Add(96 * time.Hour)

	items := []BulkItem{
		{
			Issue:  model.Issue{Number: 1, Title: "Fast fix", URL: "https://github.com/org/repo/issues/1", Labels: []string{"bug"}, ClosedAt: &closed1},
			Metric: model.Metric{Duration: &d1},
		},
		{
			Issue:  model.Issue{Number: 2, Title: "Slow fix", URL: "https://github.com/org/repo/issues/2", ClosedAt: &closed2},
			Metric: model.Metric{Duration: &d2},
		},
	}
	stats := metrics.ComputeStats([]time.Duration{d1, d2})

	var buf bytes.Buffer
	err := WriteBulkJSON(&buf, "org/repo", since, until, "issue", items, stats, "https://github.com/search?q=test", nil, nil)
	if err != nil {
		t.Fatalf("WriteBulkJSON() error: %v", err)
	}

	// Parse into the exact struct type to verify wire format
	var parsed jsonBulkOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v\nraw: %s", err, buf.String())
	}

	// Top-level fields
	if parsed.Repository != "org/repo" {
		t.Errorf("repository = %q, want %q", parsed.Repository, "org/repo")
	}
	if parsed.Strategy != "issue" {
		t.Errorf("strategy = %q, want %q", parsed.Strategy, "issue")
	}
	if parsed.SearchURL != "https://github.com/search?q=test" {
		t.Errorf("search_url = %q, want test URL", parsed.SearchURL)
	}
	if parsed.Window.Since == "" || parsed.Window.Until == "" {
		t.Error("window.since or window.until is empty")
	}
	if parsed.Stats.Count != 2 {
		t.Errorf("stats.count = %d, want 2", parsed.Stats.Count)
	}

	// Items
	if len(parsed.Items) != 2 {
		t.Fatalf("items length = %d, want 2", len(parsed.Items))
	}
	if parsed.Items[0].Number == 0 {
		t.Error("first item number is 0")
	}
	if parsed.Items[0].CycleTime.Duration == "" {
		t.Error("first item cycle_time.duration is empty")
	}

	// Verify JSON field names by parsing into a generic map
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("raw JSON parse error: %v", err)
	}
	for _, key := range []string{"repository", "window", "search_url", "strategy", "sort", "items", "stats"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing top-level JSON key %q", key)
		}
	}

	// Verify per-item field names
	var itemsRaw []map[string]json.RawMessage
	if err := json.Unmarshal(raw["items"], &itemsRaw); err != nil {
		t.Fatalf("items parse error: %v", err)
	}
	if len(itemsRaw) > 0 {
		if _, ok := itemsRaw[0]["cycle_time"]; !ok {
			t.Error("item missing 'cycle_time' field — wire format broken")
		}
		if _, ok := itemsRaw[0]["number"]; !ok {
			t.Error("item missing 'number' field")
		}
	}
}

func TestWriteBulkJSON_WithWarnings(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	var buf bytes.Buffer
	warnings := []string{"test warning"}
	err := WriteBulkJSON(&buf, "org/repo", since, until, "issue", nil, model.Stats{}, "", warnings, nil)
	if err != nil {
		t.Fatalf("WriteBulkJSON() error: %v", err)
	}

	var parsed jsonBulkOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}
	if len(parsed.Warnings) != 1 || parsed.Warnings[0] != "test warning" {
		t.Errorf("warnings = %v, want [\"test warning\"]", parsed.Warnings)
	}
}

func TestWriteIssueJSON_Structure(t *testing.T) {
	d := 24 * time.Hour
	m := model.Metric{Duration: &d}

	var buf bytes.Buffer
	err := WriteIssueJSON(&buf, "org/repo", 42, "Fix bug", "closed", "https://github.com/org/repo/issues/42", []string{"bug"}, m, nil)
	if err != nil {
		t.Fatalf("WriteIssueJSON() error: %v", err)
	}

	var parsed jsonSingleOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}
	if parsed.Repository != "org/repo" {
		t.Errorf("repository = %q, want %q", parsed.Repository, "org/repo")
	}
	if parsed.Issue != 42 {
		t.Errorf("issue = %d, want 42", parsed.Issue)
	}
	if parsed.CycleTime.Duration == "" {
		t.Error("cycle_time.duration is empty")
	}

	// Verify wire format field name
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("raw parse error: %v", err)
	}
	if _, ok := raw["cycle_time"]; !ok {
		t.Error("missing 'cycle_time' field — wire format broken")
	}
}

func TestWriteBulkJSON_FlagClassification(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	noiseD := 30 * time.Second
	hotfixD := 24 * time.Hour
	normalD := 100 * time.Hour
	closed := since.Add(48 * time.Hour)

	items := []BulkItem{
		{Issue: model.Issue{Number: 1, Title: "Noise", ClosedAt: &closed}, Metric: model.Metric{Duration: &noiseD}},
		{Issue: model.Issue{Number: 2, Title: "Hotfix", ClosedAt: &closed}, Metric: model.Metric{Duration: &hotfixD}},
		{Issue: model.Issue{Number: 3, Title: "Normal", ClosedAt: &closed}, Metric: model.Metric{Duration: &normalD}},
	}
	stats := metrics.ComputeStats([]time.Duration{noiseD, hotfixD, normalD})

	var buf bytes.Buffer
	err := WriteBulkJSON(&buf, "org/repo", since, until, "issue", items, stats, "", nil, nil)
	if err != nil {
		t.Fatalf("WriteBulkJSON() error: %v", err)
	}

	var parsed jsonBulkOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	// Items are sorted by duration desc, so order is: normal, hotfix, noise
	flagsByNumber := make(map[int][]string)
	for _, item := range parsed.Items {
		flagsByNumber[item.Number] = item.Flags
	}

	// Noise item should have "noise" flag
	if flags, ok := flagsByNumber[1]; !ok {
		t.Error("missing item #1")
	} else if !containsFlag(flags, "noise") {
		t.Errorf("item #1 flags = %v, want 'noise'", flags)
	}

	// Hotfix item should have "hotfix" flag
	if flags, ok := flagsByNumber[2]; !ok {
		t.Error("missing item #2")
	} else if !containsFlag(flags, "hotfix") {
		t.Errorf("item #2 flags = %v, want 'hotfix'", flags)
	}
}

func containsFlag(flags []string, want string) bool {
	for _, f := range flags {
		if f == want {
			return true
		}
	}
	return false
}
