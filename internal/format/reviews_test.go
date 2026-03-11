package format

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestWriteReviewsPretty_WithItems(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty, IsTTY: false, Width: 120}

	result := model.ReviewPressureResult{
		Repository: "owner/repo",
		AwaitingReview: []model.PRAwaitingReview{
			{Number: 142, Title: "Add export feature", URL: "https://github.com/owner/repo/pull/142", Age: 84 * time.Hour, IsStale: true},
			{Number: 145, Title: "Update docs", URL: "https://github.com/owner/repo/pull/145", Age: 6 * time.Hour, IsStale: false},
		},
	}

	if err := WriteReviewsPretty(rc, result); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "Review Queue: owner/repo") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "STALE") {
		t.Error("missing STALE signal")
	}
	if !strings.Contains(out, "2 PRs awaiting review (1 stale >48h)") {
		t.Errorf("missing summary, got: %s", out)
	}
}

func TestWriteReviewsPretty_Empty(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Pretty, IsTTY: false, Width: 120}

	result := model.ReviewPressureResult{
		Repository: "owner/repo",
	}

	if err := WriteReviewsPretty(rc, result); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "No PRs currently awaiting review") {
		t.Error("missing empty message")
	}
}

func TestWriteReviewsJSON(t *testing.T) {
	var buf bytes.Buffer

	result := model.ReviewPressureResult{
		Repository: "owner/repo",
		AwaitingReview: []model.PRAwaitingReview{
			{Number: 142, Title: "Add export feature", Age: 84 * time.Hour, IsStale: true},
			{Number: 145, Title: "Update docs", Age: 6 * time.Hour, IsStale: false},
		},
	}

	if err := WriteReviewsJSON(&buf, result); err != nil {
		t.Fatal(err)
	}

	var out jsonReviewsOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 {
		t.Errorf("count = %d, want 2", out.Count)
	}
	if out.StaleCount != 1 {
		t.Errorf("stale_count = %d, want 1", out.StaleCount)
	}
	if !out.Items[0].IsStale {
		t.Error("first item should be stale")
	}
}

func TestWriteReviewsMarkdown(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, Format: Markdown, IsTTY: false, Width: 120}

	result := model.ReviewPressureResult{
		Repository: "owner/repo",
		AwaitingReview: []model.PRAwaitingReview{
			{Number: 142, Title: "Add export feature", URL: "https://github.com/owner/repo/pull/142", Age: 84 * time.Hour, IsStale: true},
		},
	}

	if err := WriteReviewsMarkdown(rc, result); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "## Review Queue: owner/repo") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "[#142]") {
		t.Error("missing markdown link")
	}
	if !strings.Contains(out, "STALE") {
		t.Error("missing STALE signal")
	}
}
