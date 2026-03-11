package model

import (
	"testing"
	"time"
)

var (
	testNow   = time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	testSince = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
)

func TestIssueStatus(t *testing.T) {
	tests := []struct {
		name       string
		issue      Issue
		wantStatus string
	}{
		{
			name: "new issue created in window",
			issue: Issue{
				CreatedAt: testSince.Add(24 * time.Hour),
				UpdatedAt: testSince.Add(24 * time.Hour),
			},
			wantStatus: StatusNew,
		},
		{
			name: "stale issue no update in 10 days",
			issue: Issue{
				CreatedAt: testNow.Add(-30 * 24 * time.Hour),
				UpdatedAt: testNow.Add(-10 * 24 * time.Hour),
			},
			wantStatus: StatusStale,
		},
		{
			name: "active issue recently updated",
			issue: Issue{
				CreatedAt: testSince.Add(-5 * 24 * time.Hour),
				UpdatedAt: testNow.Add(-1 * 24 * time.Hour),
			},
			wantStatus: StatusActive,
		},
		{
			name: "borderline stale at exactly threshold",
			issue: Issue{
				CreatedAt: testNow.Add(-20 * 24 * time.Hour),
				UpdatedAt: testNow.Add(-7 * 24 * time.Hour),
			},
			wantStatus: StatusStale,
		},
		{
			name: "not stale at threshold minus 1",
			issue: Issue{
				CreatedAt: testNow.Add(-20 * 24 * time.Hour),
				UpdatedAt: testNow.Add(-6 * 24 * time.Hour),
			},
			wantStatus: StatusActive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := IssueStatus(tt.issue, testSince, testNow)
			if s.Status != tt.wantStatus {
				t.Errorf("IssueStatus().Status = %q, want %q", s.Status, tt.wantStatus)
			}
		})
	}
}

func TestPRStatus(t *testing.T) {
	tests := []struct {
		name        string
		pr          PR
		needsReview bool
		wantStatus  string
	}{
		{
			name:        "needs review after grace period",
			pr:          PR{CreatedAt: testSince.Add(-3 * 24 * time.Hour)},
			needsReview: true,
			wantStatus:  StatusNeedsReview,
		},
		{
			name:        "new PR in window even with no reviews",
			pr:          PR{CreatedAt: testSince.Add(24 * time.Hour)},
			needsReview: true,
			wantStatus:  StatusNew,
		},
		{
			name:        "active PR with reviews",
			pr:          PR{CreatedAt: testSince.Add(-1 * 24 * time.Hour)},
			needsReview: false,
			wantStatus:  StatusActive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := PRStatus(tt.pr, tt.needsReview, testSince, testNow)
			if s.Status != tt.wantStatus {
				t.Errorf("PRStatus().Status = %q, want %q", s.Status, tt.wantStatus)
			}
		})
	}
}

func TestIssueIsStale(t *testing.T) {
	stale := Issue{UpdatedAt: testNow.Add(-10 * 24 * time.Hour)}
	if !stale.IsStale(testNow) {
		t.Error("expected stale issue to be stale")
	}

	fresh := Issue{UpdatedAt: testNow.Add(-1 * 24 * time.Hour)}
	if fresh.IsStale(testNow) {
		t.Error("expected fresh issue to not be stale")
	}
}

func TestPRNeedsReview(t *testing.T) {
	pr := PR{Number: 42}
	set := []PR{{Number: 42}, {Number: 99}}
	if !PRNeedsReview(pr, set) {
		t.Error("expected PR 42 to be in needs review set")
	}

	other := PR{Number: 50}
	if PRNeedsReview(other, set) {
		t.Error("expected PR 50 to NOT be in needs review set")
	}
}

func TestDaysBetween(t *testing.T) {
	a := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	got := DaysBetween(a, b)
	if got != 7 {
		t.Errorf("DaysBetween = %d, want 7", got)
	}
}

func TestDaysBetween_NeverNegative(t *testing.T) {
	a := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) // b before a
	got := DaysBetween(a, b)
	if got != 0 {
		t.Errorf("DaysBetween = %d, want 0 (clamped)", got)
	}
}

func TestComputeInsights(t *testing.T) {
	closedMar3 := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	closedMar5 := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	closedMar7 := time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)

	mergedMar2 := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	mergedMar6 := time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)
	mergedMar4 := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)

	r := MyWeekResult{
		Login: "u",
		Repo:  "o/r",
		Since: testSince,
		Until: testNow,
		IssuesClosed: []Issue{
			// 11 days lead time
			{Number: 1, CreatedAt: time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC), ClosedAt: &closedMar3},
			// 4 days lead time
			{Number: 2, CreatedAt: testSince, ClosedAt: &closedMar5},
			// 10 days lead time
			{Number: 3, CreatedAt: time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC), ClosedAt: &closedMar7},
		},
		PRsMerged: []PR{
			// 2 days cycle time
			{Number: 10, CreatedAt: time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), MergedAt: &mergedMar2},
			// 5 days cycle time
			{Number: 11, CreatedAt: testSince, MergedAt: &mergedMar6},
			// 6 days cycle time
			{Number: 12, CreatedAt: time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC), MergedAt: &mergedMar4},
		},
		IssuesOpen: []Issue{
			// Stale
			{Number: 30, CreatedAt: testNow.Add(-20 * 24 * time.Hour), UpdatedAt: testNow.Add(-15 * 24 * time.Hour)},
			// New
			{Number: 42, CreatedAt: testSince.Add(2 * 24 * time.Hour), UpdatedAt: testSince.Add(2 * 24 * time.Hour)},
		},
		PRsOpen: []PR{
			{Number: 43, CreatedAt: testSince.Add(-3 * 24 * time.Hour)},
		},
		PRsNeedingReview: []PR{
			{Number: 43},
		},
		PRsAwaitingMyReview: []PR{
			{Number: 50, Author: "alice"},
		},
		Releases: []Release{
			{TagName: "v1.0"},
		},
	}

	ins := ComputeInsights(r)

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
	r := MyWeekResult{Login: "u", Repo: "o/r", Since: testSince, Until: testNow}
	ins := ComputeInsights(r)
	if ins.LeadTime != nil {
		t.Errorf("LeadTime = %v, want nil for empty result", *ins.LeadTime)
	}
	if ins.CycleTime != nil {
		t.Errorf("CycleTime = %v, want nil for empty result", *ins.CycleTime)
	}
}
