package metrics

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

var (
	insightsTestNow   = time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	insightsTestSince = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
)

func TestComputeInsights(t *testing.T) {
	closedMar3 := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	closedMar5 := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	closedMar7 := time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)

	mergedMar2 := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	mergedMar6 := time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)
	mergedMar4 := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)

	r := model.MyWeekResult{
		Login: "u",
		Repo:  "o/r",
		Since: insightsTestSince,
		Until: insightsTestNow,
		IssuesClosed: []model.Issue{
			// 11 days lead time
			{Number: 1, CreatedAt: time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC), ClosedAt: &closedMar3},
			// 4 days lead time
			{Number: 2, CreatedAt: insightsTestSince, ClosedAt: &closedMar5},
			// 10 days lead time
			{Number: 3, CreatedAt: time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC), ClosedAt: &closedMar7},
		},
		PRsMerged: []model.PR{
			// 2 days cycle time
			{Number: 10, CreatedAt: time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), MergedAt: &mergedMar2},
			// 5 days cycle time
			{Number: 11, CreatedAt: insightsTestSince, MergedAt: &mergedMar6},
			// 6 days cycle time
			{Number: 12, CreatedAt: time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC), MergedAt: &mergedMar4},
		},
		IssuesOpen: []model.Issue{
			// Stale
			{Number: 30, CreatedAt: insightsTestNow.Add(-20 * 24 * time.Hour), UpdatedAt: insightsTestNow.Add(-15 * 24 * time.Hour)},
			// New
			{Number: 42, CreatedAt: insightsTestSince.Add(2 * 24 * time.Hour), UpdatedAt: insightsTestSince.Add(2 * 24 * time.Hour)},
		},
		PRsOpen: []model.PR{
			{Number: 43, CreatedAt: insightsTestSince.Add(-3 * 24 * time.Hour)},
		},
		PRsNeedingReview: []model.PR{
			{Number: 43},
		},
		PRsAwaitingMyReview: []model.PR{
			{Number: 50, Author: "alice"},
		},
		Releases: []model.Release{
			{TagName: "v1.0"},
		},
	}

	// Pre-compute cycle time durations (PR strategy: created → merged)
	var cycleTimeDurations []time.Duration
	for _, pr := range r.PRsMerged {
		if pr.MergedAt != nil {
			d := pr.MergedAt.Sub(pr.CreatedAt)
			if d > 0 {
				cycleTimeDurations = append(cycleTimeDurations, d)
			}
		}
	}

	ins := ComputeInsights(r, cycleTimeDurations)

	if ins.StaleIssues != 1 {
		t.Errorf("StaleIssues = %d, want 1", ins.StaleIssues)
	}
	if ins.NewIssues != 1 {
		t.Errorf("NewIssues = %d, want 1", ins.NewIssues)
	}
	if ins.PRsNeedingReview != 1 {
		t.Errorf("PRsNeedingReview = %d, want 1", ins.PRsNeedingReview)
	}
	if ins.PRsAwaitingMyReview != 1 {
		t.Errorf("PRsAwaitingMyReview = %d, want 1", ins.PRsAwaitingMyReview)
	}
	if ins.Releases != 1 {
		t.Errorf("Releases = %d, want 1", ins.Releases)
	}

	// Lead time: sorted [4d, 10d, 11d], median = 10d = 240h
	if ins.LeadTime == nil {
		t.Fatal("LeadTime is nil, want non-nil")
	}
	leadDays := int(ins.LeadTime.Hours() / 24)
	if leadDays != 10 {
		t.Errorf("LeadTime = %v (%d days), want ~10 days", *ins.LeadTime, leadDays)
	}

	// Cycle time: sorted [2d, 5d, 6d], median = 5d = 120h
	if ins.CycleTime == nil {
		t.Fatal("CycleTime is nil, want non-nil")
	}
	cycleDays := int(ins.CycleTime.Hours() / 24)
	if cycleDays != 5 {
		t.Errorf("CycleTime = %v (%d days), want ~5 days", *ins.CycleTime, cycleDays)
	}
}

func TestComputeInsights_Empty(t *testing.T) {
	r := model.MyWeekResult{Login: "u", Repo: "o/r", Since: insightsTestSince, Until: insightsTestNow}
	ins := ComputeInsights(r, nil)
	if ins.LeadTime != nil {
		t.Errorf("LeadTime = %v, want nil for empty result", *ins.LeadTime)
	}
	if ins.CycleTime != nil {
		t.Errorf("CycleTime = %v, want nil for empty result", *ins.CycleTime)
	}
}
