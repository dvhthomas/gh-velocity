package throughput

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestProcessData(t *testing.T) {
	p := &Pipeline{
		Owner:      "org",
		Repo:       "repo",
		Since:      time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Until:      time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
		issueCount: 5,
		prCount:    3,
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	if p.Result.Repository != "org/repo" {
		t.Errorf("Repository = %q, want org/repo", p.Result.Repository)
	}
	if p.Result.IssuesClosed != 5 {
		t.Errorf("IssuesClosed = %d, want 5", p.Result.IssuesClosed)
	}
	if p.Result.PRsMerged != 3 {
		t.Errorf("PRsMerged = %d, want 3", p.Result.PRsMerged)
	}
}

func TestProcessData_ZeroCounts(t *testing.T) {
	p := &Pipeline{
		Owner: "org",
		Repo:  "repo",
		Since: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	if p.Result.IssuesClosed != 0 {
		t.Errorf("IssuesClosed = %d, want 0", p.Result.IssuesClosed)
	}
	if p.Result.PRsMerged != 0 {
		t.Errorf("PRsMerged = %d, want 0", p.Result.PRsMerged)
	}
}

func TestRender_JSON(t *testing.T) {
	p := &Pipeline{
		SearchURL: "https://github.com/search?q=test",
		Result: model.ThroughputResult{
			Repository:   "org/repo",
			Since:        time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Until:        time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
			IssuesClosed: 5,
			PRsMerged:    3,
		},
	}

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.JSON}
	if err := p.Render(rc); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	var parsed jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if parsed.Issues != 5 {
		t.Errorf("issues_closed = %d, want 5", parsed.Issues)
	}
	if parsed.PRs != 3 {
		t.Errorf("prs_merged = %d, want 3", parsed.PRs)
	}
	if parsed.Total != 8 {
		t.Errorf("total = %d, want 8", parsed.Total)
	}
	if parsed.SearchURL != "https://github.com/search?q=test" {
		t.Errorf("search_url = %q, want test URL", parsed.SearchURL)
	}
}

func TestRender_Pretty(t *testing.T) {
	p := &Pipeline{
		Result: model.ThroughputResult{
			Repository:   "org/repo",
			Since:        time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Until:        time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
			IssuesClosed: 5,
			PRsMerged:    3,
		},
	}

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Pretty}
	if err := p.Render(rc); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"org/repo", "Issues closed: 5", "PRs merged:    3", "Total:         8"} {
		if !containsStr(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestRender_Pretty_EmptyWithVerifyURL(t *testing.T) {
	p := &Pipeline{
		SearchURL: "https://github.com/search?q=test",
		Result: model.ThroughputResult{
			Repository: "org/repo",
			Since:      time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			Until:      time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, Format: format.Pretty}
	if err := p.Render(rc); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	out := buf.String()
	if !containsStr(out, "No activity") {
		t.Error("expected 'No activity' for empty result")
	}
	if !containsStr(out, "Verify: https://github.com/search?q=test") {
		t.Error("expected verify URL for empty result")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
