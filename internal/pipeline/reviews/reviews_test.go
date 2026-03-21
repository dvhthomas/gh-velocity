package reviews

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestWritePretty_WithItems(t *testing.T) {
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Pretty, IsTTY: false, Width: 120}

	result := model.ReviewPressureResult{
		Repository: "owner/repo",
		AwaitingReview: []model.PRAwaitingReview{
			{Number: 142, Title: "Add export feature", URL: "https://github.com/owner/repo/pull/142", Age: 84 * time.Hour, IsStale: true},
			{Number: 145, Title: "Update docs", URL: "https://github.com/owner/repo/pull/145", Age: 6 * time.Hour, IsStale: false},
		},
	}

	if err := WritePretty(rc, result, ""); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "Review Queue: owner/repo") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "⏳") {
		t.Error("missing stale flag emoji ⏳")
	}
	if !strings.Contains(out, "2 PRs awaiting review (1 stale >48h)") {
		t.Errorf("missing summary, got: %s", out)
	}
}

func TestWritePretty_Empty(t *testing.T) {
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Pretty, IsTTY: false, Width: 120}

	result := model.ReviewPressureResult{
		Repository: "owner/repo",
	}

	if err := WritePretty(rc, result, ""); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "No PRs currently awaiting review") {
		t.Error("missing empty message")
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer

	result := model.ReviewPressureResult{
		Repository: "owner/repo",
		AwaitingReview: []model.PRAwaitingReview{
			{Number: 142, Title: "Add export feature", Age: 84 * time.Hour, IsStale: true},
			{Number: 145, Title: "Update docs", Age: 6 * time.Hour, IsStale: false},
		},
	}

	if err := WriteJSON(&buf, result, "", nil); err != nil {
		t.Fatal(err)
	}

	var out jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 {
		t.Errorf("count = %d, want 2", out.Count)
	}
	if out.StaleCount != 1 {
		t.Errorf("stale_count = %d, want 1", out.StaleCount)
	}
	if len(out.Items[0].Flags) == 0 || out.Items[0].Flags[0] != "stale" {
		t.Errorf("first item flags = %v, want [stale]", out.Items[0].Flags)
	}
}

func TestWriteMarkdown(t *testing.T) {
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Markdown, IsTTY: false, Width: 120}

	result := model.ReviewPressureResult{
		Repository: "owner/repo",
		AwaitingReview: []model.PRAwaitingReview{
			{Number: 142, Title: "Add export feature", URL: "https://github.com/owner/repo/pull/142", Age: 84 * time.Hour, IsStale: true},
		},
	}

	if err := WriteMarkdown(rc, result, ""); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "## Review Queue: owner/repo") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "[#142]") {
		t.Error("missing markdown link")
	}
	if !strings.Contains(out, "⏳") {
		t.Error("missing stale flag emoji ⏳")
	}
}
