package metrics

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/git"
)

func TestComputeBusFactor(t *testing.T) {
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

	result := ComputeBusFactor(paths, since, 2)

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

func TestComputeBusFactor_Empty(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := ComputeBusFactor(nil, since, 2)
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
