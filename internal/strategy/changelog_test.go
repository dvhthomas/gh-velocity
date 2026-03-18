package strategy

import (
	"context"
	"testing"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestChangelog_ParsesReferences(t *testing.T) {
	release := &model.Release{
		Body: `## What's Changed
* Fix login timeout (#42)
* Add new dashboard #15
* Update dependencies (#3)
`,
	}

	s := NewChangelog()
	items, err := s.Discover(context.Background(), DiscoverInput{Release: release})
	if err != nil {
		t.Fatal(err)
	}

	found := make(map[int]bool)
	for _, item := range items {
		if item.Issue != nil {
			found[item.Issue.Number] = true
		}
	}

	if !found[42] || !found[15] || !found[3] {
		t.Errorf("expected issues 42, 15, 3; found: %v", found)
	}
}

func TestChangelog_NoDuplicates(t *testing.T) {
	release := &model.Release{
		Body: "Fixes #10, also see #10 for context",
	}

	s := NewChangelog()
	items, err := s.Discover(context.Background(), DiscoverInput{Release: release})
	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 1 {
		t.Errorf("expected 1 unique item, got %d", len(items))
	}
}

func TestChangelog_EmptyBody(t *testing.T) {
	release := &model.Release{Body: ""}

	s := NewChangelog()
	items, err := s.Discover(context.Background(), DiscoverInput{Release: release})
	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestChangelog_NilRelease(t *testing.T) {
	s := NewChangelog()
	items, err := s.Discover(context.Background(), DiscoverInput{Release: nil})
	if err != nil {
		t.Fatal(err)
	}

	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestChangelog_StrategyName(t *testing.T) {
	s := NewChangelog()
	if s.Name() != "changelog" {
		t.Errorf("expected name 'changelog', got %q", s.Name())
	}
}
