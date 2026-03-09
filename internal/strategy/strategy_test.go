package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

func TestRunner_TimeWindowGuardrail(t *testing.T) {
	runner := NewRunner(31) // 31-day max

	prevDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tagDate := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) // 59 days later

	input := DiscoverInput{
		Tag:         "v1.1.0",
		PreviousTag: "v1.0.0",
		TagDate:     tagDate,
		PrevTagDate: prevDate,
	}

	_, _, err := runner.Run(context.Background(), input)
	if err == nil {
		t.Fatal("expected time window error, got nil")
	}
}

func TestRunner_TimeWindowOK(t *testing.T) {
	// Use a mock strategy that returns nothing.
	runner := NewRunner(31, &mockStrategy{name: "test"})

	prevDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tagDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC) // 14 days later

	input := DiscoverInput{
		Tag:         "v1.1.0",
		PreviousTag: "v1.0.0",
		TagDate:     tagDate,
		PrevTagDate: prevDate,
	}

	result, _, err := runner.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestRunner_MaxWindowDaysCapped(t *testing.T) {
	runner := NewRunner(200) // exceeds HardMaxWindowDays
	if runner.maxWindowDays != HardMaxWindowDays {
		t.Errorf("expected max %d, got %d", HardMaxWindowDays, runner.maxWindowDays)
	}
}

func TestRunner_DefaultMaxWindowDays(t *testing.T) {
	runner := NewRunner(0)
	if runner.maxWindowDays != DefaultMaxWindowDays {
		t.Errorf("expected default %d, got %d", DefaultMaxWindowDays, runner.maxWindowDays)
	}
}

func TestRunner_StrategyErrorBecomeWarning(t *testing.T) {
	failing := &mockStrategy{name: "failing", err: true}
	passing := &mockStrategy{name: "passing", items: []model.DiscoveredItem{
		{Issue: &model.Issue{Number: 1}, Strategy: "passing"},
	}}
	runner := NewRunner(31, failing, passing)

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
