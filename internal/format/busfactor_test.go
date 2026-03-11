package format

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/git"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
)

func testBusFactorResult() metrics.BusFactorResult {
	return metrics.BusFactorResult{
		Since: time.Now().Add(-90 * 24 * time.Hour),
		Depth: 2,
		Paths: []metrics.PathRisk{
			{
				Path:             "internal/strategy/",
				Risk:             metrics.RiskHigh,
				ContributorCount: 1,
				Primary:          git.Contributor{Name: "Alice", Email: "alice@test.com", Commits: 47},
				PrimaryPct:       100,
				TotalCommits:     47,
			},
			{
				Path:             "internal/github/",
				Risk:             metrics.RiskMedium,
				ContributorCount: 2,
				Primary:          git.Contributor{Name: "Bob", Email: "bob@test.com", Commits: 122},
				PrimaryPct:       78.2,
				TotalCommits:     156,
			},
			{
				Path:             "cmd/",
				Risk:             metrics.RiskLow,
				ContributorCount: 3,
				Primary:          git.Contributor{Name: "Alice", Email: "alice@test.com", Commits: 25},
				PrimaryPct:       40.3,
				TotalCommits:     62,
			},
		},
	}
}

func TestWriteBusFactorPretty(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, IsTTY: false, Width: 120}
	result := testBusFactorResult()

	if err := WriteBusFactorPretty(rc, result); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{
		"Knowledge Risk Report",
		"HIGH",
		"MEDIUM",
		"LOW",
		"internal/strategy/",
		"Alice (100%)",
		"Bob (78%)",
		"distributed",
		"Summary: 1 HIGH risk, 1 MEDIUM risk, 1 LOW risk areas",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("pretty output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestWriteBusFactorMarkdown(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, IsTTY: false, Width: 120}
	result := testBusFactorResult()

	if err := WriteBusFactorMarkdown(rc, result); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, want := range []string{
		"## Knowledge Risk Report",
		"| Risk | Path |",
		"| HIGH |",
		"| MEDIUM |",
		"| LOW |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestWriteBusFactorJSON(t *testing.T) {
	var buf bytes.Buffer
	result := testBusFactorResult()

	if err := WriteBusFactorJSON(&buf, result); err != nil {
		t.Fatal(err)
	}

	var out jsonBusFactorOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nraw:\n%s", err, buf.String())
	}

	if len(out.Paths) != 3 {
		t.Errorf("expected 3 paths, got %d", len(out.Paths))
	}
	if out.Depth != 2 {
		t.Errorf("depth = %d, want 2", out.Depth)
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

	// JSON should include emails (privacy: only in JSON).
	if out.Paths[0].Primary.Email != "alice@test.com" {
		t.Errorf("JSON path[0] email = %q, want alice@test.com", out.Paths[0].Primary.Email)
	}
}

func TestWriteBusFactorPretty_Empty(t *testing.T) {
	var buf bytes.Buffer
	rc := RenderContext{Writer: &buf, IsTTY: false, Width: 120}
	result := metrics.BusFactorResult{
		Since: time.Now().Add(-90 * 24 * time.Hour),
		Depth: 2,
	}

	if err := WriteBusFactorPretty(rc, result); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "No paths with enough commit activity") {
		t.Errorf("empty output missing message, got:\n%s", buf.String())
	}
}
