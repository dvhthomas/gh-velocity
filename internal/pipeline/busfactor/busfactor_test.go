package busfactor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/git"
)

// ============================================================
// Compute tests (from metrics/busfactor_test.go)
// ============================================================

func TestCompute(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	paths := []git.PathContributors{
		{
			Path: "internal/strategy/",
			Contributors: []git.Contributor{
				{Name: "Alice", Email: "alice@test.com", Commits: 47},
			},
			TotalCommits: 47,
		},
		{
			Path: "internal/github/",
			Contributors: []git.Contributor{
				{Name: "Bob", Email: "bob@test.com", Commits: 122},
				{Name: "Alice", Email: "alice@test.com", Commits: 34},
			},
			TotalCommits: 156,
		},
		{
			Path: "internal/metrics/",
			Contributors: []git.Contributor{
				{Name: "Alice", Email: "alice@test.com", Commits: 30},
				{Name: "Bob", Email: "bob@test.com", Commits: 25},
				{Name: "Carol", Email: "carol@test.com", Commits: 20},
				{Name: "Dave", Email: "dave@test.com", Commits: 14},
			},
			TotalCommits: 89,
		},
		{
			Path: "cmd/",
			Contributors: []git.Contributor{
				{Name: "Alice", Email: "alice@test.com", Commits: 25},
				{Name: "Bob", Email: "bob@test.com", Commits: 20},
				{Name: "Carol", Email: "carol@test.com", Commits: 17},
			},
			TotalCommits: 62,
		},
	}

	result := Compute(paths, since, 2, 5)

	if len(result.Paths) != 4 {
		t.Fatalf("expected 4 paths, got %d", len(result.Paths))
	}

	// Should be sorted: HIGH first, then MEDIUM, then LOW.
	if result.Paths[0].Risk != RiskHigh {
		t.Errorf("first path risk = %s, want HIGH", result.Paths[0].Risk)
	}
	if result.Paths[0].Path != "internal/strategy/" {
		t.Errorf("first path = %s, want internal/strategy/", result.Paths[0].Path)
	}
	if result.Paths[0].ContributorCount != 1 {
		t.Errorf("first path contributors = %d, want 1", result.Paths[0].ContributorCount)
	}

	if result.Paths[1].Risk != RiskMedium {
		t.Errorf("second path risk = %s, want MEDIUM", result.Paths[1].Risk)
	}
	if result.Paths[1].Path != "internal/github/" {
		t.Errorf("second path = %s, want internal/github/", result.Paths[1].Path)
	}

	// LOW risk paths should be sorted alphabetically.
	if result.Paths[2].Risk != RiskLow {
		t.Errorf("third path risk = %s, want LOW", result.Paths[2].Risk)
	}
	if result.Paths[3].Risk != RiskLow {
		t.Errorf("fourth path risk = %s, want LOW", result.Paths[3].Risk)
	}

	// Check primary percentage for HIGH risk path.
	if result.Paths[0].PrimaryPct != 100 {
		t.Errorf("HIGH risk primary pct = %.1f, want 100", result.Paths[0].PrimaryPct)
	}
}

func TestCompute_Empty(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := Compute(nil, since, 2, 5)
	if len(result.Paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(result.Paths))
	}
}

func TestClassifyRisk(t *testing.T) {
	tests := []struct {
		name       string
		count      int
		primaryPct float64
		want       RiskLevel
	}{
		{"solo contributor", 1, 100, RiskHigh},
		{"two contributors primary dominant", 2, 80, RiskMedium},
		{"two contributors balanced", 2, 55, RiskLow},
		{"two contributors at boundary", 2, 70, RiskLow},
		{"two contributors above boundary", 2, 71, RiskMedium},
		{"three contributors", 3, 90, RiskLow},
		{"many contributors", 5, 50, RiskLow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRisk(tt.count, tt.primaryPct)
			if got != tt.want {
				t.Errorf("classifyRisk(%d, %.0f) = %s, want %s", tt.count, tt.primaryPct, got, tt.want)
			}
		})
	}
}

// ============================================================
// Pipeline ProcessData test (from metric/busfactor_test.go)
// ============================================================

func TestPipelineProcessData(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	p := &Pipeline{
		Repository: "org/repo",
		Since:      since,
		Depth:      2,
		MinCommits: 5,
		paths: []git.PathContributors{
			{
				Path: "cmd",
				Contributors: []git.Contributor{
					{Name: "Alice", Email: "alice@example.com", Commits: 20},
				},
				TotalCommits: 20,
			},
			{
				Path: "internal/format",
				Contributors: []git.Contributor{
					{Name: "Alice", Email: "alice@example.com", Commits: 15},
					{Name: "Bob", Email: "bob@example.com", Commits: 3},
				},
				TotalCommits: 18,
			},
			{
				Path: "internal/model",
				Contributors: []git.Contributor{
					{Name: "Alice", Email: "alice@example.com", Commits: 5},
					{Name: "Bob", Email: "bob@example.com", Commits: 5},
					{Name: "Carol", Email: "carol@example.com", Commits: 5},
				},
				TotalCommits: 15,
			},
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData() error: %v", err)
	}

	if p.Result.Repository != "org/repo" {
		t.Errorf("Repository = %q, want %q", p.Result.Repository, "org/repo")
	}

	if len(p.Result.Paths) != 3 {
		t.Fatalf("got %d paths, want 3", len(p.Result.Paths))
	}

	// Paths should be sorted: HIGH first, then MEDIUM, then LOW
	wantRisks := []RiskLevel{RiskHigh, RiskMedium, RiskLow}
	for i, want := range wantRisks {
		if p.Result.Paths[i].Risk != want {
			t.Errorf("Paths[%d].Risk = %s, want %s", i, p.Result.Paths[i].Risk, want)
		}
	}

	if p.Result.Paths[0].Path != "cmd" {
		t.Errorf("HIGH risk path = %q, want %q", p.Result.Paths[0].Path, "cmd")
	}
	if p.Result.Paths[1].Path != "internal/format" {
		t.Errorf("MEDIUM risk path = %q, want %q", p.Result.Paths[1].Path, "internal/format")
	}
	if p.Result.Paths[2].Path != "internal/model" {
		t.Errorf("LOW risk path = %q, want %q", p.Result.Paths[2].Path, "internal/model")
	}
}

// ============================================================
// Render tests (from format/busfactor_test.go)
// ============================================================

func testResult() Result {
	return Result{
		Repository: "acme/widgets",
		Since:      time.Now().Add(-90 * 24 * time.Hour),
		Depth:      2,
		MinCommits: 5,
		Paths: []PathRisk{
			{
				Path:             "internal/strategy/",
				Risk:             RiskHigh,
				ContributorCount: 1,
				Primary:          git.Contributor{Name: "Alice", Email: "alice@test.com", Commits: 47},
				PrimaryPct:       100,
				TotalCommits:     47,
			},
			{
				Path:             "internal/github/",
				Risk:             RiskMedium,
				ContributorCount: 2,
				Primary:          git.Contributor{Name: "Bob", Email: "bob@test.com", Commits: 122},
				PrimaryPct:       78.2,
				TotalCommits:     156,
			},
			{
				Path:             "cmd/",
				Risk:             RiskLow,
				ContributorCount: 3,
				Primary:          git.Contributor{Name: "Alice", Email: "alice@test.com", Commits: 25},
				PrimaryPct:       40.3,
				TotalCommits:     62,
			},
		},
	}
}

func TestWritePretty(t *testing.T) {
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false, Width: 120}
	result := testResult()

	if err := WritePretty(rc, result); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{
		"Knowledge Risk Report: acme/widgets",
		"min-commits 5",
		"HIGH",
		"MEDIUM",
		"LOW",
		"internal/strategy/",
		"Alice (100%)",
		"Bob (78%)",
		"distributed",
		"1 HIGH risk, 1 MEDIUM risk, 1 LOW risk areas",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("pretty output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestWriteMarkdown(t *testing.T) {
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false, Width: 120}
	result := testResult()

	if err := WriteMarkdown(rc, result); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{
		"<!-- gh-velocity:bus-factor repo=acme/widgets",
		"## Knowledge Risk Report: acme/widgets",
		"depth=2",
		"min-commits=5",
		"| Risk | Path |",
		"| **HIGH** | `internal/strategy/`",
		"| **MEDIUM** |",
		"| **LOW** |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	result := testResult()

	if err := WriteJSON(&buf, result, nil); err != nil {
		t.Fatal(err)
	}

	var out jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nraw:\n%s", err, buf.String())
	}

	if len(out.Paths) != 3 {
		t.Errorf("expected 3 paths, got %d", len(out.Paths))
	}
	if out.Repository != "acme/widgets" {
		t.Errorf("repository = %q, want acme/widgets", out.Repository)
	}
	if out.Depth != 2 {
		t.Errorf("depth = %d, want 2", out.Depth)
	}
	if out.MinCommits != 5 {
		t.Errorf("min_commits = %d, want 5", out.MinCommits)
	}
	if out.Summary.High != 1 {
		t.Errorf("summary.high = %d, want 1", out.Summary.High)
	}
	if out.Summary.Medium != 1 {
		t.Errorf("summary.medium = %d, want 1", out.Summary.Medium)
	}
	if out.Summary.Low != 1 {
		t.Errorf("summary.low = %d, want 1", out.Summary.Low)
	}

	if out.Paths[0].Primary.Email != "alice@test.com" {
		t.Errorf("JSON path[0] email = %q, want alice@test.com", out.Paths[0].Primary.Email)
	}
}

func TestWritePretty_Empty(t *testing.T) {
	var buf bytes.Buffer
	rc := format.RenderContext{Writer: &buf, IsTTY: false, Width: 120}
	result := Result{
		Since: time.Now().Add(-90 * 24 * time.Hour),
		Depth: 2,
	}

	if err := WritePretty(rc, result); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "No paths with enough commit activity") {
		t.Errorf("empty output missing message, got:\n%s", buf.String())
	}
}
