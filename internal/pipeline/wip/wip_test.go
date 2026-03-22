package wip

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/config"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// mockSearcher implements the searcher interface for testing.
type mockSearcher struct {
	issues     map[string][]model.Issue // query -> issues
	prs        map[string][]model.PR    // query -> prs
	issueErr   map[string]error
	prErr      map[string]error
	issueCalls []string
	prCalls    []string
}

func (m *mockSearcher) SearchIssues(_ context.Context, query string) ([]model.Issue, error) {
	m.issueCalls = append(m.issueCalls, query)
	if err, ok := m.issueErr[query]; ok {
		return nil, err
	}
	return m.issues[query], nil
}

func (m *mockSearcher) SearchPRs(_ context.Context, query string) ([]model.PR, error) {
	m.prCalls = append(m.prCalls, query)
	if err, ok := m.prErr[query]; ok {
		return nil, err
	}
	return m.prs[query], nil
}

func TestGatherData_Injected(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	issues := []model.Issue{
		{Number: 1, Title: "issue 1", Labels: []string{"in-progress"}, CreatedAt: now.Add(-48 * time.Hour)},
	}
	prs := []model.PR{
		{Number: 10, Title: "pr 1", Draft: true, CreatedAt: now.Add(-24 * time.Hour)},
	}

	p := &Pipeline{
		Owner:          "owner",
		Repo:           "repo",
		InjectedIssues: issues,
		InjectedPRs:    prs,
		Now:            now,
		LifecycleConfig: config.LifecycleConfig{
			InProgress: config.LifecycleStage{Match: []string{"label:in-progress"}},
		},
		EffortConfig: config.EffortConfig{Strategy: "count"},
	}

	if err := p.GatherData(context.Background()); err != nil {
		t.Fatalf("GatherData error: %v", err)
	}

	if len(p.OpenIssues) != 1 {
		t.Errorf("openIssues = %d, want 1", len(p.OpenIssues))
	}
	if len(p.openPRs) != 1 {
		t.Errorf("openPRs = %d, want 1", len(p.openPRs))
	}
}

func TestGatherData_Standalone(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	issue1 := model.Issue{Number: 1, Title: "issue 1", Labels: []string{"in-progress"}, CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now}
	pr1 := model.PR{Number: 10, Title: "pr 1", Labels: []string{"in-review"}, CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now}
	unlabeledPR := model.PR{Number: 20, Title: "unlabeled pr", CreatedAt: now.Add(-12 * time.Hour), UpdatedAt: now}

	mock := &mockSearcher{
		issues: map[string][]model.Issue{
			`repo:owner/repo is:open is:issue label:"in-progress"`: {issue1},
			`repo:owner/repo is:open is:issue label:"in-review"`:   {},
		},
		prs: map[string][]model.PR{
			`repo:owner/repo is:open is:pr label:"in-progress"`: {},
			`repo:owner/repo is:open is:pr label:"in-review"`:   {pr1},
			`repo:owner/repo is:open is:pr no:label`:            {unlabeledPR},
		},
	}

	p := &Pipeline{
		Client: mock,
		Owner:  "owner",
		Repo:   "repo",
		Now:    now,
		LifecycleConfig: config.LifecycleConfig{
			InProgress: config.LifecycleStage{Match: []string{"label:in-progress"}},
			InReview:   config.LifecycleStage{Match: []string{"label:in-review"}},
		},
		EffortConfig: config.EffortConfig{Strategy: "count"},
	}

	if err := p.GatherData(context.Background()); err != nil {
		t.Fatalf("GatherData error: %v", err)
	}

	if len(p.OpenIssues) != 1 {
		t.Errorf("openIssues = %d, want 1", len(p.OpenIssues))
	}
	if len(p.openPRs) != 2 {
		t.Errorf("openPRs = %d, want 2", len(p.openPRs))
	}
}

func TestGatherData_DeduplicatesIssues(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	// Same issue appears in both label searches.
	issue1 := model.Issue{Number: 1, Title: "issue 1", Labels: []string{"in-progress", "wip"}, CreatedAt: now, UpdatedAt: now}

	mock := &mockSearcher{
		issues: map[string][]model.Issue{
			`repo:o/r is:open is:issue label:"in-progress"`: {issue1},
			`repo:o/r is:open is:issue label:"wip"`:         {issue1},
		},
		prs: map[string][]model.PR{
			`repo:o/r is:open is:pr label:"in-progress"`: {},
			`repo:o/r is:open is:pr label:"wip"`:         {},
			`repo:o/r is:open is:pr no:label`:            {},
		},
	}

	p := &Pipeline{
		Client: mock,
		Owner:  "o",
		Repo:   "r",
		Now:    now,
		LifecycleConfig: config.LifecycleConfig{
			InProgress: config.LifecycleStage{Match: []string{"label:in-progress", "label:wip"}},
		},
		EffortConfig: config.EffortConfig{Strategy: "count"},
	}

	if err := p.GatherData(context.Background()); err != nil {
		t.Fatalf("GatherData error: %v", err)
	}

	if len(p.OpenIssues) != 1 {
		t.Errorf("openIssues = %d, want 1 (should deduplicate)", len(p.OpenIssues))
	}
}

func TestGatherData_PartialFailure(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	issue1 := model.Issue{Number: 1, Title: "issue 1", Labels: []string{"in-progress"}, CreatedAt: now, UpdatedAt: now}

	mock := &mockSearcher{
		issues: map[string][]model.Issue{
			`repo:o/r is:open is:issue label:"in-progress"`: {issue1},
		},
		issueErr: map[string]error{
			`repo:o/r is:open is:issue label:"wip"`: fmt.Errorf("rate limited"),
		},
		prs: map[string][]model.PR{
			`repo:o/r is:open is:pr label:"in-progress"`: {},
			`repo:o/r is:open is:pr label:"wip"`:         {},
			`repo:o/r is:open is:pr no:label`:            {},
		},
	}

	var warnings []string
	p := &Pipeline{
		Client: mock,
		Owner:  "o",
		Repo:   "r",
		Now:    now,
		LifecycleConfig: config.LifecycleConfig{
			InProgress: config.LifecycleStage{Match: []string{"label:in-progress", "label:wip"}},
		},
		EffortConfig: config.EffortConfig{Strategy: "count"},
		WarnFunc:     func(f string, a ...any) { warnings = append(warnings, fmt.Sprintf(f, a...)) },
	}

	// Should not return error — partial failure is OK.
	if err := p.GatherData(context.Background()); err != nil {
		t.Fatalf("GatherData error: %v", err)
	}

	if len(p.OpenIssues) != 1 {
		t.Errorf("openIssues = %d, want 1", len(p.OpenIssues))
	}
	if len(p.Warnings()) == 0 {
		t.Error("expected warnings for failed search")
	}
}

func TestProcessData_InjectedWithoutGatherData(t *testing.T) {
	t.Parallel()

	// Regression: cmd/report.go sets InjectedIssues/InjectedPRs and calls
	// ProcessData() directly (without GatherData). ProcessData must use the
	// injected data — not empty slices.
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

	p := &Pipeline{
		Owner: "owner",
		Repo:  "repo",
		Now:   now,
		LifecycleConfig: config.LifecycleConfig{
			InProgress: config.LifecycleStage{Match: []string{"label:in-progress"}},
		},
		EffortConfig: config.EffortConfig{Strategy: "count"},
		WIPConfig:    config.WIPConfig{},
		InjectedIssues: []model.Issue{
			{
				Number:    1,
				Title:     "Active issue",
				Labels:    []string{"in-progress"},
				Assignees: []string{"alice"},
				CreatedAt: now.Add(-48 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
		},
		InjectedPRs: []model.PR{
			{
				Number:    10,
				Title:     "Open PR",
				Labels:    []string{},
				Author:    "bob",
				Draft:     false,
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now,
			},
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData error: %v", err)
	}

	if len(p.Result.Items) == 0 {
		t.Fatal("ProcessData with InjectedIssues/InjectedPRs produced 0 WIP items; want >0")
	}

	// Should find the in-progress issue
	found := false
	for _, item := range p.Result.Items {
		if item.Number == 1 && item.Status == "In Progress" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected issue #1 classified as In Progress; got items: %v", p.Result.Items)
	}
}

func TestProcessData(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

	p := &Pipeline{
		Owner: "owner",
		Repo:  "repo",
		Now:   now,
		LifecycleConfig: config.LifecycleConfig{
			InProgress: config.LifecycleStage{Match: []string{"label:in-progress"}},
			InReview:   config.LifecycleStage{Match: []string{"label:in-review"}},
		},
		EffortConfig: config.EffortConfig{Strategy: "count"},
		WIPConfig:    config.WIPConfig{},
		OpenIssues: []model.Issue{
			{
				Number:    1,
				Title:     "Active issue",
				Labels:    []string{"in-progress"},
				Assignees: []string{"alice"},
				CreatedAt: now.Add(-48 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
			{
				Number:    2,
				Title:     "Review issue",
				Labels:    []string{"in-review"},
				Assignees: []string{"bob"},
				CreatedAt: now.Add(-72 * time.Hour),
				UpdatedAt: now.Add(-5 * 24 * time.Hour),
			},
			{
				Number:    3,
				Title:     "Unrelated issue",
				Labels:    []string{"enhancement"},
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now,
			},
		},
		openPRs: []model.PR{
			{
				Number:    10,
				Title:     "Draft PR",
				Labels:    []string{},
				Assignees: []string{"alice"},
				Draft:     true,
				CreatedAt: now.Add(-12 * time.Hour),
				UpdatedAt: now,
			},
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData error: %v", err)
	}

	// Issue #3 should be excluded (no matcher match, not a PR).
	if len(p.Result.Items) != 3 {
		t.Fatalf("items = %d, want 3 (issue#1, issue#2, pr#10)", len(p.Result.Items))
	}

	// Check stage counts.
	if len(p.Result.StageCounts) == 0 {
		t.Fatal("expected stage counts")
	}

	// Check assignees.
	if len(p.Result.Assignees) == 0 {
		t.Fatal("expected assignees")
	}

	// Alice should have 2 items (issue #1 + PR #10).
	var aliceFound bool
	for _, a := range p.Result.Assignees {
		if a.Login == "alice" {
			aliceFound = true
			if a.ItemCount != 2 {
				t.Errorf("alice ItemCount = %d, want 2", a.ItemCount)
			}
		}
	}
	if !aliceFound {
		t.Error("alice not found in assignees")
	}

	// Check insights were generated.
	if len(p.Result.Insights) == 0 {
		t.Error("expected insights")
	}

	// Check repository.
	if p.Result.Repository != "owner/repo" {
		t.Errorf("repository = %q, want %q", p.Result.Repository, "owner/repo")
	}
}

func TestProcessData_WIPLimits(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	teamLimit := 2.0
	personLimit := 1.0

	p := &Pipeline{
		Owner: "owner",
		Repo:  "repo",
		Now:   now,
		LifecycleConfig: config.LifecycleConfig{
			InProgress: config.LifecycleStage{Match: []string{"label:in-progress"}},
		},
		EffortConfig: config.EffortConfig{Strategy: "count"},
		WIPConfig: config.WIPConfig{
			TeamLimit:   &teamLimit,
			PersonLimit: &personLimit,
		},
		OpenIssues: []model.Issue{
			{Number: 1, Labels: []string{"in-progress"}, Assignees: []string{"alice"}, CreatedAt: now, UpdatedAt: now},
			{Number: 2, Labels: []string{"in-progress"}, Assignees: []string{"alice"}, CreatedAt: now, UpdatedAt: now},
			{Number: 3, Labels: []string{"in-progress"}, Assignees: []string{"bob"}, CreatedAt: now, UpdatedAt: now},
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData error: %v", err)
	}

	// Team limit: 3 items > 2 limit.
	if p.Result.TeamLimit == nil || *p.Result.TeamLimit != 2.0 {
		t.Error("expected team limit to be set")
	}

	// Warnings should include team limit exceeded (now references "human WIP").
	var hasTeamWarning, hasPersonWarning bool
	for _, w := range p.Warnings() {
		if contains(w, "human WIP exceeds team limit") {
			hasTeamWarning = true
		}
		if contains(w, "alice") && contains(w, "person WIP limit") {
			hasPersonWarning = true
		}
	}
	if !hasTeamWarning {
		t.Error("expected human WIP exceeds team limit warning")
	}
	if !hasPersonWarning {
		t.Error("expected person WIP limit warning for alice")
	}

	// Alice should be marked over limit.
	for _, a := range p.Result.Assignees {
		if a.Login == "alice" && !a.OverLimit {
			t.Error("expected alice to be over limit")
		}
		if a.Login == "bob" && a.OverLimit {
			t.Error("bob should not be over limit")
		}
	}
}

func TestProcessData_BotSplit(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)

	p := &Pipeline{
		Owner: "owner",
		Repo:  "repo",
		Now:   now,
		LifecycleConfig: config.LifecycleConfig{
			InProgress: config.LifecycleStage{Match: []string{"label:in-progress"}},
		},
		EffortConfig: config.EffortConfig{Strategy: "count"},
		WIPConfig:    config.WIPConfig{},
		OpenIssues: []model.Issue{
			{Number: 1, Labels: []string{"in-progress"}, Assignees: []string{"alice"}, CreatedAt: now, UpdatedAt: now},
			{Number: 2, Labels: []string{"in-progress"}, Assignees: []string{"dependabot[bot]"}, CreatedAt: now, UpdatedAt: now},
			{Number: 3, Labels: []string{"in-progress"}, Assignees: []string{"bob"}, CreatedAt: now, UpdatedAt: now},
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData error: %v", err)
	}

	// 3 total items: 2 human, 1 bot.
	if p.Result.HumanItemCount != 2 {
		t.Errorf("HumanItemCount = %d, want 2", p.Result.HumanItemCount)
	}
	if p.Result.BotItemCount != 1 {
		t.Errorf("BotItemCount = %d, want 1", p.Result.BotItemCount)
	}

	// Human assignees should not include dependabot.
	for _, a := range p.Result.Assignees {
		if a.Login == "dependabot[bot]" {
			t.Error("dependabot[bot] should not be in human Assignees")
		}
	}

	// Bot assignees should include dependabot.
	var foundBot bool
	for _, a := range p.Result.BotAssignees {
		if a.Login == "dependabot[bot]" {
			foundBot = true
		}
	}
	if !foundBot {
		t.Error("dependabot[bot] should be in BotAssignees")
	}

	// Effort split.
	if p.Result.HumanEffort != 2 {
		t.Errorf("HumanEffort = %f, want 2", p.Result.HumanEffort)
	}
	if p.Result.BotEffort != 1 {
		t.Errorf("BotEffort = %f, want 1", p.Result.BotEffort)
	}
}

func TestProcessData_WIPLimitsApplyToHumanOnly(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	teamLimit := 3.0

	p := &Pipeline{
		Owner: "owner",
		Repo:  "repo",
		Now:   now,
		LifecycleConfig: config.LifecycleConfig{
			InProgress: config.LifecycleStage{Match: []string{"label:in-progress"}},
		},
		EffortConfig: config.EffortConfig{Strategy: "count"},
		WIPConfig:    config.WIPConfig{TeamLimit: &teamLimit},
		OpenIssues: []model.Issue{
			{Number: 1, Labels: []string{"in-progress"}, Assignees: []string{"alice"}, CreatedAt: now, UpdatedAt: now},
			{Number: 2, Labels: []string{"in-progress"}, Assignees: []string{"alice"}, CreatedAt: now, UpdatedAt: now},
			{Number: 3, Labels: []string{"in-progress"}, Assignees: []string{"dependabot[bot]"}, CreatedAt: now, UpdatedAt: now},
			{Number: 4, Labels: []string{"in-progress"}, Assignees: []string{"dependabot[bot]"}, CreatedAt: now, UpdatedAt: now},
		},
	}

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData error: %v", err)
	}

	// Total effort is 4 (exceeds limit 3), but human effort is 2 (within limit 3).
	// No team limit warning should be emitted.
	for _, w := range p.Warnings() {
		if contains(w, "team limit") {
			t.Errorf("should not warn about team limit when human effort (%f) is within limit, got: %s",
				p.Result.HumanEffort, w)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
