package format

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "0s"},
		{"seconds", 42 * time.Second, "42s"},
		{"minutes", 28 * time.Minute, "28m"},
		{"hours and minutes", 10*time.Hour + 43*time.Minute, "10h 43m"},
		{"days and hours", 3*24*time.Hour + 13*time.Hour, "3d 13h"},
		{"negative", -2 * time.Hour, "-2h 0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.d)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestFormatDurationPtr(t *testing.T) {
	d := 5 * time.Hour
	if got := FormatDurationPtr(&d); got != "5h 0m" {
		t.Errorf("got %q", got)
	}
	if got := FormatDurationPtr(nil); got != "N/A" {
		t.Errorf("got %q for nil, want N/A", got)
	}
}

func TestParseFormat(t *testing.T) {
	for _, valid := range []string{"json", "pretty", "markdown"} {
		f, err := ParseFormat(valid)
		if err != nil {
			t.Errorf("ParseFormat(%q) unexpected error: %v", valid, err)
		}
		if string(f) != valid {
			t.Errorf("got %q, want %q", f, valid)
		}
	}

	_, err := ParseFormat("csv")
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestWriteReportPretty_CycleTimeNA_IssueStrategy(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty}
	r := model.StatsResult{
		Repository:        "owner/repo",
		Since:             time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Until:             time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
		CycleTimeStrategy: model.StrategyIssue,
	}
	if err := WriteReportPretty(rc, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "not available") {
		t.Error("expected 'not available' for nil cycle time with issue strategy")
	}
	if !strings.Contains(out, "lifecycle.in-progress.project_status") {
		t.Error("expected project_status hint")
	}
}

func TestWriteReportPretty_CycleTimeNA_PRStrategy(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty}
	r := model.StatsResult{
		Repository:        "owner/repo",
		Since:             time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Until:             time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
		CycleTimeStrategy: model.StrategyPR,
	}
	if err := WriteReportPretty(rc, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "not available") {
		t.Error("expected 'not available' for nil cycle time with pr strategy")
	}
	if !strings.Contains(out, "closing PRs") {
		t.Error("expected closing PRs hint")
	}
}

func TestSanitizeMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"pipe escaped", "feat: add | operator", "feat: add \\| operator"},
		{"html stripped", "title <script>alert('xss')</script>", "title "},
		{"newlines removed", "line1\nline2\r\nline3", "line1 line2 line3"},
		{"multiple pipes", "| DROP TABLE |", "\\| DROP TABLE \\|"},
		{"html link stripped", `<a href="http://evil.com">click me</a>`, "click me"},
		{"truncated", strings.Repeat("a", 250), strings.Repeat("a", 197) + "..."},
		{"short text unchanged", "hello world", "hello world"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
