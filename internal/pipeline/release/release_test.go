package release

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestWriteJSON(t *testing.T) {
	lt := 48 * time.Hour
	ct := 24 * time.Hour
	rl := 12 * time.Hour
	cadence := 7 * 24 * time.Hour
	meanLT := 48 * time.Hour
	medLT := 48 * time.Hour

	now := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	rm := model.ReleaseMetrics{
		Tag:            "v1.0.0",
		PreviousTag:    "v0.9.0",
		Date:           time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC),
		Cadence:        &cadence,
		TotalIssues:    1,
		CategoryNames:  []string{"bug", "feature", "other"},
		CategoryCounts: map[string]int{"bug": 0, "feature": 1, "other": 0},
		CategoryRatios: map[string]float64{"bug": 0, "feature": 1.0, "other": 0},
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
	if err := WriteJSON(&buf, "owner/repo", rm, nil); err != nil {
		t.Fatal(err)
	}

	var out jsonReleaseOutput
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

func TestWriteJSON_WithWarnings(t *testing.T) {
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
	if err := WriteJSON(&buf, "owner/repo", rm, warnings); err != nil {
		t.Fatal(err)
	}

	var out jsonReleaseOutput
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

func TestWriteMarkdown(t *testing.T) {
	rm := model.ReleaseMetrics{
		Tag:         "v1.0.0",
		TotalIssues: 0,
	}

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Markdown}
	if err := WriteMarkdown(rc, rm, nil); err != nil {
		t.Fatal(err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty markdown output")
	}
}
