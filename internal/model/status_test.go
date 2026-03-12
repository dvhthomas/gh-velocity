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

