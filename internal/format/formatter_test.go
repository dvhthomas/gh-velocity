package format

import (
	"bytes"
	"encoding/json"
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

func TestWriteReleaseJSON(t *testing.T) {
	lt := 48 * time.Hour
	ct := 24 * time.Hour
	rl := 12 * time.Hour
	cadence := 7 * 24 * time.Hour
	meanLT := 48 * time.Hour
	medLT := 48 * time.Hour

	now := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	rm := model.ReleaseMetrics{
		Tag:          "v1.0.0",
		PreviousTag:  "v0.9.0",
		Date:         now,
		Cadence:      &cadence,
		TotalIssues:  1,
		CategoryCounts: map[string]int{"feature": 1},
		CategoryRatios: map[string]float64{"feature": 1.0},
		Issues: []model.IssueMetrics{
			{
				Issue:       model.Issue{Number: 1, Title: "Add feature"},
				LeadTime:    model.Metric{Start: &model.Event{Time: now, Signal: "issue-created"}, End: &model.Event{Time: now.Add(lt), Signal: "issue-closed"}, Duration: &lt},
				CycleTime:   model.Metric{Start: &model.Event{Time: now, Signal: "commit"}, End: &model.Event{Time: now.Add(ct), Signal: "issue-closed"}, Duration: &ct},
				ReleaseLag:  model.Metric{Start: &model.Event{Time: now, Signal: "issue-closed"}, End: &model.Event{Time: now.Add(rl), Signal: "release-published"}, Duration: &rl},
				CommitCount: 3,
			},
		},
		LeadTimeStats: model.Stats{Count: 1, Mean: &meanLT, Median: &medLT},
	}

	var buf bytes.Buffer
	if err := WriteReleaseJSON(&buf, "owner/repo", rm, nil); err != nil {
		t.Fatal(err)
	}

	var out JSONReleaseOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if out.Tag != "v1.0.0" {
		t.Errorf("tag: want v1.0.0, got %s", out.Tag)
	}
	if len(out.Issues) != 1 {
		t.Errorf("issues: want 1, got %d", len(out.Issues))
	}
}

func TestWriteReleaseJSON_WithWarnings(t *testing.T) {
	rm := model.ReleaseMetrics{
		Tag:         "v1.0.0",
		TotalIssues: 1,
		Issues: []model.IssueMetrics{
			{
				Issue:       model.Issue{Number: 1, Title: "Working issue"},
				CommitCount: 1,
			},
		},
	}

	warnings := []string{
		"skipped issue #99: not found",
		"1 issue(s) skipped due to fetch errors",
	}

	var buf bytes.Buffer
	if err := WriteReleaseJSON(&buf, "owner/repo", rm, warnings); err != nil {
		t.Fatal(err)
	}

	var out JSONReleaseOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(out.Warnings) != 2 {
		t.Errorf("expected 2 warnings in JSON output, got %d", len(out.Warnings))
	}
	if len(out.Issues) != 1 {
		t.Errorf("expected 1 issue in JSON output, got %d", len(out.Issues))
	}
}

func TestWriteReleaseMarkdown(t *testing.T) {
	rm := model.ReleaseMetrics{
		Tag:         "v1.0.0",
		TotalIssues: 0,
	}

	var buf bytes.Buffer
	if err := WriteReleaseMarkdown(&buf, rm, nil); err != nil {
		t.Fatal(err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty markdown output")
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
			got := sanitizeMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
