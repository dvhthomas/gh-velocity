package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestCommitRef_ClosingKeywordsOnly(t *testing.T) {
	commits := []model.Commit{
		{SHA: "a1", Message: "fixes #42", AuthoredAt: time.Now()},
		{SHA: "a2", Message: "update step #1", AuthoredAt: time.Now()}, // bare ref, should NOT match
		{SHA: "a3", Message: "closes #10", AuthoredAt: time.Now()},
		{SHA: "a4", Message: "resolves #42", AuthoredAt: time.Now()}, // duplicate of #42
	}

	s := NewCommitRef(nil) // default: closes only
	items, err := s.Discover(context.Background(), DiscoverInput{Commits: commits})
	if err != nil {
		t.Fatal(err)
	}

	// Should find #42 and #10, but NOT #1 (bare ref not matching by default)
	found := make(map[int]bool)
	for _, item := range items {
		if item.Issue != nil {
			found[item.Issue.Number] = true
		}
	}

	if !found[42] {
		t.Error("expected to find issue #42")
	}
	if !found[10] {
		t.Error("expected to find issue #10")
	}
	if found[1] {
		t.Error("bare #1 should NOT match with default (closes-only) patterns")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestCommitRef_WithRefsPattern(t *testing.T) {
	commits := []model.Commit{
		{SHA: "a1", Message: "fixes #42", AuthoredAt: time.Now()},
		{SHA: "a2", Message: "update step #1", AuthoredAt: time.Now()},
		{SHA: "a3", Message: "closes #10", AuthoredAt: time.Now()},
	}

	s := NewCommitRef([]string{"closes", "refs"})
	items, err := s.Discover(context.Background(), DiscoverInput{Commits: commits})
	if err != nil {
		t.Fatal(err)
	}

	found := make(map[int]bool)
	for _, item := range items {
		if item.Issue != nil {
			found[item.Issue.Number] = true
		}
	}

	if !found[42] {
		t.Error("expected to find issue #42")
	}
	if !found[10] {
		t.Error("expected to find issue #10")
	}
	if !found[1] {
		t.Error("expected to find issue #1 with refs pattern enabled")
	}
}

func TestCommitRef_CommitsGroupedByIssue(t *testing.T) {
	commits := []model.Commit{
		{SHA: "a1", Message: "fixes #5 initial work", AuthoredAt: time.Now()},
		{SHA: "a2", Message: "fixes #5 follow up", AuthoredAt: time.Now()},
	}

	s := NewCommitRef(nil)
	items, err := s.Discover(context.Background(), DiscoverInput{Commits: commits})
	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item for issue #5, got %d", len(items))
	}
	if len(items[0].Commits) != 2 {
		t.Errorf("expected 2 commits for issue #5, got %d", len(items[0].Commits))
	}
}

func TestCommitRef_StrategyName(t *testing.T) {
	s := NewCommitRef(nil)
	if s.Name() != "commit-ref" {
		t.Errorf("expected name 'commit-ref', got %q", s.Name())
	}
}

func TestCommitRef_EmptyCommits(t *testing.T) {
	s := NewCommitRef(nil)
	items, err := s.Discover(context.Background(), DiscoverInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestCommitRef_CaseInsensitiveKeywords(t *testing.T) {
	commits := []model.Commit{
		{SHA: "a1", Message: "FIXES #7", AuthoredAt: time.Now()},
		{SHA: "a2", Message: "Closes #8", AuthoredAt: time.Now()},
		{SHA: "a3", Message: "Resolved #9", AuthoredAt: time.Now()},
	}

	s := NewCommitRef(nil)
	items, err := s.Discover(context.Background(), DiscoverInput{Commits: commits})
	if err != nil {
		t.Fatal(err)
	}

	found := make(map[int]bool)
	for _, item := range items {
		if item.Issue != nil {
			found[item.Issue.Number] = true
		}
	}

	if !found[7] || !found[8] || !found[9] {
		t.Errorf("expected to find issues 7, 8, 9; found: %v", found)
	}
}
