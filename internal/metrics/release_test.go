package metrics

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/classify"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// testClassifier creates a Classifier from bug/feature label names for test convenience.
func testClassifier(t *testing.T, bugLabels, featureLabels []string) *classify.Classifier {
	t.Helper()
	var cats []model.CategoryConfig
	if len(bugLabels) > 0 {
		matchers := make([]string, len(bugLabels))
		for i, l := range bugLabels {
			matchers[i] = "label:" + l
		}
		cats = append(cats, model.CategoryConfig{Name: "bug", Matchers: matchers})
	}
	if len(featureLabels) > 0 {
		matchers := make([]string, len(featureLabels))
		for i, l := range featureLabels {
			matchers[i] = "label:" + l
		}
		cats = append(cats, model.CategoryConfig{Name: "feature", Matchers: matchers})
	}
	c, err := classify.NewClassifier(cats)
	if err != nil {
		t.Fatalf("testClassifier: %v", err)
	}
	return c
}

func TestBuildReleaseMetrics_Empty(t *testing.T) {
	input := ReleaseInput{
		Tag:          "v1.0.0",
		Release:      model.Release{TagName: "v1.0.0", CreatedAt: time.Now()},
		IssueCommits: map[int][]model.Commit{},
		Issues:       map[int]*model.Issue{},
		FetchErrors:  map[int]error{},
		Classifier:   testClassifier(t, []string{"bug"}, []string{"enhancement"}),
	}

	rm, warnings, err := BuildReleaseMetrics(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
	if rm.TotalIssues != 0 {
		t.Errorf("expected 0 issues, got %d", rm.TotalIssues)
	}
	if rm.Tag != "v1.0.0" {
		t.Errorf("expected tag v1.0.0, got %s", rm.Tag)
	}
}

func TestBuildReleaseMetrics_Classification(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	closed := now.Add(-24 * time.Hour)

	issues := map[int]*model.Issue{
		1: {Number: 1, Labels: []string{"bug"}, CreatedAt: now.Add(-72 * time.Hour), ClosedAt: &closed},
		2: {Number: 2, Labels: []string{"enhancement"}, CreatedAt: now.Add(-48 * time.Hour), ClosedAt: &closed},
		3: {Number: 3, Labels: []string{"docs"}, CreatedAt: now.Add(-24 * time.Hour), ClosedAt: &closed},
		4: {Number: 4, Labels: []string{"bug", "urgent"}, CreatedAt: now.Add(-96 * time.Hour), ClosedAt: &closed},
	}

	issueCommits := map[int][]model.Commit{
		1: {{SHA: "aaaaaaa", AuthoredAt: now.Add(-60 * time.Hour)}},
		2: {{SHA: "bbbbbbb", AuthoredAt: now.Add(-40 * time.Hour)}},
		3: {{SHA: "ccccccc", AuthoredAt: now.Add(-20 * time.Hour)}},
		4: {{SHA: "ddddddd", AuthoredAt: now.Add(-80 * time.Hour)}},
	}

	input := ReleaseInput{
		Tag:          "v1.0.0",
		Release:      model.Release{TagName: "v1.0.0", CreatedAt: now},
		IssueCommits: issueCommits,
		Issues:       issues,
		FetchErrors:  map[int]error{},
		Classifier:   testClassifier(t, []string{"bug"}, []string{"enhancement"}),
	}

	rm, _, err := BuildReleaseMetrics(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rm.TotalIssues != 4 {
		t.Errorf("expected 4 issues, got %d", rm.TotalIssues)
	}
	if rm.CategoryCounts["bug"] != 2 {
		t.Errorf("expected 2 bugs, got %d", rm.CategoryCounts["bug"])
	}
	if rm.CategoryCounts["feature"] != 1 {
		t.Errorf("expected 1 feature, got %d", rm.CategoryCounts["feature"])
	}
	if rm.CategoryCounts["other"] != 1 {
		t.Errorf("expected 1 other, got %d", rm.CategoryCounts["other"])
	}
	if rm.CategoryRatios["bug"] != 0.5 {
		t.Errorf("expected bug ratio 0.5, got %f", rm.CategoryRatios["bug"])
	}
	if rm.CategoryRatios["feature"] != 0.25 {
		t.Errorf("expected feature ratio 0.25, got %f", rm.CategoryRatios["feature"])
	}
	if rm.CategoryRatios["other"] != 0.25 {
		t.Errorf("expected other ratio 0.25, got %f", rm.CategoryRatios["other"])
	}
}

func TestBuildReleaseMetrics_LeadTimeAndCycleTime(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	created := now.Add(-72 * time.Hour)
	closed := now.Add(-24 * time.Hour)
	commitTime := now.Add(-48 * time.Hour)

	issues := map[int]*model.Issue{
		1: {Number: 1, Labels: []string{"bug"}, CreatedAt: created, ClosedAt: &closed},
	}
	issueCommits := map[int][]model.Commit{
		1: {{SHA: "abcdefg", AuthoredAt: commitTime}},
	}

	input := ReleaseInput{
		Tag:               "v1.0.0",
		Release:           model.Release{TagName: "v1.0.0", CreatedAt: now},
		IssueCommits:      issueCommits,
		Issues:            issues,
		FetchErrors:       map[int]error{},
		Classifier:        testClassifier(t, []string{"bug"}, []string{"enhancement"}),
		CycleTimeStrategy: &IssueStrategy{},
	}

	rm, _, err := BuildReleaseMetrics(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rm.Issues) != 1 {
		t.Fatalf("expected 1 issue metric, got %d", len(rm.Issues))
	}

	im := rm.Issues[0]

	// Lead time: created -> closed = 48h
	expectedLT := 48 * time.Hour
	if im.LeadTime.Duration == nil || *im.LeadTime.Duration != expectedLT {
		t.Errorf("expected lead time %v, got %v", expectedLT, im.LeadTime.Duration)
	}
	if im.LeadTime.Start == nil || im.LeadTime.Start.Signal != model.SignalIssueCreated {
		t.Error("expected lead time start signal to be issue-created")
	}
	if im.LeadTime.End == nil || im.LeadTime.End.Signal != model.SignalIssueClosed {
		t.Error("expected lead time end signal to be issue-closed")
	}

	// Cycle time with issue strategy (no project): nil — signal unavailable
	if im.CycleTime.Duration != nil {
		t.Errorf("expected nil cycle time (no project configured), got %v", *im.CycleTime.Duration)
	}

	// Release lag: closed -> release = 24h
	expectedLag := 24 * time.Hour
	if im.ReleaseLag.Duration == nil || *im.ReleaseLag.Duration != expectedLag {
		t.Errorf("expected release lag %v, got %v", expectedLag, im.ReleaseLag.Duration)
	}
	if im.ReleaseLag.End == nil || im.ReleaseLag.End.Signal != model.SignalReleasePublished {
		t.Error("expected release lag end signal to be release-published")
	}

	// Stats should have count=1
	if rm.LeadTimeStats.Count != 1 {
		t.Errorf("expected lead time stats count 1, got %d", rm.LeadTimeStats.Count)
	}
}

func TestBuildReleaseMetrics_FetchErrorsAsWarnings(t *testing.T) {
	now := time.Now()
	input := ReleaseInput{
		Tag:     "v1.0.0",
		Release: model.Release{TagName: "v1.0.0", CreatedAt: now},
		IssueCommits: map[int][]model.Commit{
			1: {{SHA: "abcdefg", AuthoredAt: now}},
			2: {{SHA: "defghij", AuthoredAt: now}},
		},
		Issues: map[int]*model.Issue{
			1: {Number: 1, Labels: []string{"bug"}, CreatedAt: now},
		},
		FetchErrors: map[int]error{2: fmt.Errorf("not found")},
		Classifier:  testClassifier(t, []string{"bug"}, []string{"enhancement"}),
	}

	rm, warnings, err := BuildReleaseMetrics(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rm.TotalIssues != 1 {
		t.Errorf("expected 1 issue (skipped failed fetch), got %d", rm.TotalIssues)
	}
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings for fetch error (per-issue + summary), got %d: %v", len(warnings), warnings)
	}
	foundSkipped := false
	foundSummary := false
	for _, w := range warnings {
		if strings.Contains(w, "skipped issue #2") {
			foundSkipped = true
		}
		if strings.Contains(w, "1 issue(s) skipped due to fetch errors") {
			foundSummary = true
		}
	}
	if !foundSkipped {
		t.Errorf("expected per-issue skip warning, got: %v", warnings)
	}
	if !foundSummary {
		t.Errorf("expected summary skip warning, got: %v", warnings)
	}
}

func TestBuildReleaseMetrics_LowClassificationCoverageWarning(t *testing.T) {
	now := time.Now()
	closed := now.Add(-time.Hour)

	// 3 issues, all unlabeled -> >50% threshold triggers warning
	issues := map[int]*model.Issue{
		1: {Number: 1, Labels: []string{}, CreatedAt: now, ClosedAt: &closed},
		2: {Number: 2, Labels: nil, CreatedAt: now, ClosedAt: &closed},
		3: {Number: 3, Labels: []string{"random"}, CreatedAt: now, ClosedAt: &closed},
	}
	issueCommits := map[int][]model.Commit{
		1: {{SHA: "aaaaaaa", AuthoredAt: now}},
		2: {{SHA: "bbbbbbb", AuthoredAt: now}},
		3: {{SHA: "ccccccc", AuthoredAt: now}},
	}

	input := ReleaseInput{
		Tag:          "v1.0.0",
		Release:      model.Release{TagName: "v1.0.0", CreatedAt: now},
		IssueCommits: issueCommits,
		Issues:       issues,
		FetchErrors:  map[int]error{},
		Classifier:   testClassifier(t, []string{"bug"}, []string{"enhancement"}),
	}

	_, warnings, err := BuildReleaseMetrics(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "Low classification coverage") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected low classification coverage warning, got: %v", warnings)
	}
}

func TestBuildReleaseMetrics_HotfixDetection(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	input := ReleaseInput{
		Tag:               "v1.0.1",
		PreviousTag:       "v1.0.0",
		Release:           model.Release{TagName: "v1.0.1", CreatedAt: base.Add(24 * time.Hour)},
		PrevRelease:       &model.Release{TagName: "v1.0.0", CreatedAt: base},
		IssueCommits:      map[int][]model.Commit{},
		Issues:            map[int]*model.Issue{},
		FetchErrors:       map[int]error{},
		Classifier:        testClassifier(t, []string{"bug"}, []string{"enhancement"}),
		HotfixWindowHours: 72,
	}

	rm, _, err := BuildReleaseMetrics(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rm.IsHotfix {
		t.Error("expected release to be detected as hotfix")
	}
	if rm.Cadence == nil {
		t.Fatal("expected cadence to be set")
	}
	expectedCadence := 24 * time.Hour
	if *rm.Cadence != expectedCadence {
		t.Errorf("expected cadence %v, got %v", expectedCadence, *rm.Cadence)
	}
}

func TestBuildReleaseMetrics_NotHotfix(t *testing.T) {
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	input := ReleaseInput{
		Tag:               "v1.1.0",
		PreviousTag:       "v1.0.0",
		Release:           model.Release{TagName: "v1.1.0", CreatedAt: base.Add(7 * 24 * time.Hour)},
		PrevRelease:       &model.Release{TagName: "v1.0.0", CreatedAt: base},
		IssueCommits:      map[int][]model.Commit{},
		Issues:            map[int]*model.Issue{},
		FetchErrors:       map[int]error{},
		Classifier:        testClassifier(t, []string{"bug"}, []string{"enhancement"}),
		HotfixWindowHours: 72,
	}

	rm, _, err := BuildReleaseMetrics(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rm.IsHotfix {
		t.Error("expected release NOT to be detected as hotfix")
	}
}

func TestBuildReleaseMetrics_OpenIssueNoLeadTime(t *testing.T) {
	now := time.Now()

	issues := map[int]*model.Issue{
		1: {Number: 1, State: "open", Labels: []string{"bug"}, CreatedAt: now.Add(-48 * time.Hour)},
	}
	issueCommits := map[int][]model.Commit{
		1: {{SHA: "abcdefg", AuthoredAt: now.Add(-24 * time.Hour)}},
	}

	input := ReleaseInput{
		Tag:          "v1.0.0",
		Release:      model.Release{TagName: "v1.0.0", CreatedAt: now},
		IssueCommits: issueCommits,
		Issues:       issues,
		FetchErrors:  map[int]error{},
		Classifier:   testClassifier(t, []string{"bug"}, []string{"enhancement"}),
	}

	rm, _, err := BuildReleaseMetrics(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rm.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(rm.Issues))
	}
	if rm.Issues[0].LeadTime.Duration != nil {
		t.Error("expected nil lead time duration for open issue")
	}
	if rm.Issues[0].ReleaseLag.Duration != nil {
		t.Error("expected nil release lag duration for open issue (no closed date)")
	}
}
