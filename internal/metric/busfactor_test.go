package metric

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/git"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
)

func TestBusFactorProcessData(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	p := &BusFactorPipeline{
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
	wantRisks := []metrics.RiskLevel{metrics.RiskHigh, metrics.RiskMedium, metrics.RiskLow}
	for i, want := range wantRisks {
		if p.Result.Paths[i].Risk != want {
			t.Errorf("Paths[%d].Risk = %s, want %s", i, p.Result.Paths[i].Risk, want)
		}
	}

	// HIGH risk: single contributor
	if p.Result.Paths[0].Path != "cmd" {
		t.Errorf("HIGH risk path = %q, want %q", p.Result.Paths[0].Path, "cmd")
	}

	// MEDIUM risk: 2 contributors, primary >70%
	if p.Result.Paths[1].Path != "internal/format" {
		t.Errorf("MEDIUM risk path = %q, want %q", p.Result.Paths[1].Path, "internal/format")
	}

	// LOW risk: 3 contributors, distributed
	if p.Result.Paths[2].Path != "internal/model" {
		t.Errorf("LOW risk path = %q, want %q", p.Result.Paths[2].Path, "internal/model")
	}
}
