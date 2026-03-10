package strategy

import (
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestMerge_EmptyInput(t *testing.T) {
	result := Merge(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 items, got %d", len(result))
	}
}

func TestMerge_SingleStrategy(t *testing.T) {
	items := []model.DiscoveredItem{
		{Issue: &model.Issue{Number: 1}, Strategy: "pr-link"},
		{Issue: &model.Issue{Number: 2}, Strategy: "pr-link"},
	}
	results := []model.StrategyResult{
		{Name: "pr-link", Items: items},
	}

	merged := Merge(results)
	if len(merged) != 2 {
		t.Fatalf("expected 2 items, got %d", len(merged))
	}
	if merged[0].Issue.Number != 1 || merged[1].Issue.Number != 2 {
		t.Errorf("unexpected order: %d, %d", merged[0].Issue.Number, merged[1].Issue.Number)
	}
}

func TestMerge_PRLinkWinsOverCommitRef(t *testing.T) {
	now := time.Now()
	mergedAt := now.Add(-time.Hour)

	prLinkItems := []model.DiscoveredItem{
		{
			Issue:    &model.Issue{Number: 42, Title: "from pr-link"},
			PR:       &model.PR{Number: 100, MergedAt: &mergedAt},
			Strategy: "pr-link",
		},
	}
	commitRefItems := []model.DiscoveredItem{
		{
			Issue:    &model.Issue{Number: 42, Title: "from commit-ref"},
			Strategy: "commit-ref",
		},
	}

	results := []model.StrategyResult{
		{Name: "commit-ref", Items: commitRefItems},
		{Name: "pr-link", Items: prLinkItems},
	}

	merged := Merge(results)
	if len(merged) != 1 {
		t.Fatalf("expected 1 item (deduplicated), got %d", len(merged))
	}
	if merged[0].Issue.Title != "from pr-link" {
		t.Errorf("expected pr-link to win, got title %q", merged[0].Issue.Title)
	}
	if merged[0].PR == nil {
		t.Error("expected PR data to be preserved")
	}
}

func TestMerge_UniqueItemsFromDifferentStrategies(t *testing.T) {
	prLinkItems := []model.DiscoveredItem{
		{Issue: &model.Issue{Number: 1}, Strategy: "pr-link"},
	}
	commitRefItems := []model.DiscoveredItem{
		{Issue: &model.Issue{Number: 2}, Strategy: "commit-ref"},
	}
	changelogItems := []model.DiscoveredItem{
		{Issue: &model.Issue{Number: 3}, Strategy: "changelog"},
	}

	results := []model.StrategyResult{
		{Name: "changelog", Items: changelogItems},
		{Name: "commit-ref", Items: commitRefItems},
		{Name: "pr-link", Items: prLinkItems},
	}

	merged := Merge(results)
	if len(merged) != 3 {
		t.Fatalf("expected 3 unique items, got %d", len(merged))
	}
}

func TestMerge_PROnlyItems(t *testing.T) {
	items := []model.DiscoveredItem{
		{PR: &model.PR{Number: 50}, Strategy: "pr-link"},
	}
	results := []model.StrategyResult{
		{Name: "pr-link", Items: items},
	}

	merged := Merge(results)
	if len(merged) != 1 {
		t.Fatalf("expected 1 item, got %d", len(merged))
	}
	if merged[0].PR.Number != 50 {
		t.Errorf("expected PR #50, got #%d", merged[0].PR.Number)
	}
}

func TestMerge_CommitsUnionedOnDuplicate(t *testing.T) {
	now := time.Now()
	mergedAt := now.Add(-time.Hour)

	prLinkItems := []model.DiscoveredItem{
		{
			Issue:    &model.Issue{Number: 42},
			PR:       &model.PR{Number: 100, MergedAt: &mergedAt},
			Commits:  []model.Commit{{SHA: "aaa"}, {SHA: "bbb"}},
			Strategy: "pr-link",
		},
	}
	commitRefItems := []model.DiscoveredItem{
		{
			Issue:    &model.Issue{Number: 42},
			Commits:  []model.Commit{{SHA: "bbb"}, {SHA: "ccc"}},
			Strategy: "commit-ref",
		},
	}

	results := []model.StrategyResult{
		{Name: "commit-ref", Items: commitRefItems},
		{Name: "pr-link", Items: prLinkItems},
	}

	merged := Merge(results)
	if len(merged) != 1 {
		t.Fatalf("expected 1 item, got %d", len(merged))
	}
	// PR data should come from pr-link (higher priority).
	if merged[0].PR == nil || merged[0].PR.Number != 100 {
		t.Error("expected PR data from pr-link")
	}
	// Commits should be unioned: aaa, bbb, ccc (bbb deduplicated).
	if len(merged[0].Commits) != 3 {
		t.Errorf("expected 3 commits (union), got %d", len(merged[0].Commits))
	}
	shas := make(map[string]bool)
	for _, c := range merged[0].Commits {
		shas[c.SHA] = true
	}
	for _, want := range []string{"aaa", "bbb", "ccc"} {
		if !shas[want] {
			t.Errorf("missing commit %s in merged result", want)
		}
	}
}

func TestMerge_SortedByNumber(t *testing.T) {
	items := []model.DiscoveredItem{
		{Issue: &model.Issue{Number: 30}, Strategy: "pr-link"},
		{Issue: &model.Issue{Number: 10}, Strategy: "pr-link"},
		{Issue: &model.Issue{Number: 20}, Strategy: "pr-link"},
	}
	results := []model.StrategyResult{
		{Name: "pr-link", Items: items},
	}

	merged := Merge(results)
	if len(merged) != 3 {
		t.Fatalf("expected 3 items, got %d", len(merged))
	}
	for i := 0; i < len(merged)-1; i++ {
		if itemNumber(merged[i]) > itemNumber(merged[i+1]) {
			t.Errorf("items not sorted: %d > %d", itemNumber(merged[i]), itemNumber(merged[i+1]))
		}
	}
}
