package strategy

import (
	"context"
	"testing"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestRunner_StrategyErrorBecomeWarning(t *testing.T) {
	failing := &mockStrategy{name: "failing", err: true}
	passing := &mockStrategy{name: "passing", items: []model.DiscoveredItem{
		{Issue: &model.Issue{Number: 1}, Strategy: "passing"},
	}}
	runner := NewRunner(failing, passing)

	input := DiscoverInput{
		Tag:         "v1.1.0",
		PreviousTag: "v1.0.0",
	}

	result, warnings, err := runner.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(warnings))
	}
	if len(result.Merged) != 1 {
		t.Errorf("expected 1 merged item, got %d", len(result.Merged))
	}
}

func TestRunner_EmptyStrategies(t *testing.T) {
	runner := NewRunner()
	result, _, err := runner.Run(context.Background(), DiscoverInput{Tag: "v1.0.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Merged) != 0 {
		t.Errorf("expected 0 merged items, got %d", len(result.Merged))
	}
}

// mockStrategy is a test helper implementing Strategy.
type mockStrategy struct {
	name  string
	items []model.DiscoveredItem
	err   bool
}

func (m *mockStrategy) Name() string { return m.name }

func (m *mockStrategy) Discover(ctx context.Context, input DiscoverInput) ([]model.DiscoveredItem, error) {
	if m.err {
		return nil, context.DeadlineExceeded
	}
	return m.items, nil
}
