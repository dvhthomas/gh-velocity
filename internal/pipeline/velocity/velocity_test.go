package velocity

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/config"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func TestProcessData_CountFixed(t *testing.T) {
	now := date(2026, 3, 10)
	fp, _ := NewFixedPeriod(config.FixedIterationConfig{
		Length: "14d",
		Anchor: "2026-01-06",
	}, now)

	p := &Pipeline{
		Owner: "test", Repo: "repo",
		Config: config.VelocityConfig{
			Unit:      "issues",
			Effort:    config.EffortConfig{Strategy: "count"},
			Iteration: config.IterationConfig{Strategy: "fixed", Count: 3},
		},
		Now:            now,
		IterationCount: 3,
		periods:        fp,
		items: []model.VelocityItem{
			// Current iteration (Mar 3 – Mar 17): 2 closed
			{Number: 10, ContentType: "Issue", State: "closed", StateReason: "completed", ClosedAt: timePtr(date(2026, 3, 5))},
			{Number: 11, ContentType: "Issue", State: "closed", StateReason: "completed", ClosedAt: timePtr(date(2026, 3, 8))},
			// Previous iteration (Feb 17 – Mar 3): 3 closed, 1 not planned
			{Number: 7, ContentType: "Issue", State: "closed", StateReason: "completed", ClosedAt: timePtr(date(2026, 2, 20))},
			{Number: 8, ContentType: "Issue", State: "closed", StateReason: "completed", ClosedAt: timePtr(date(2026, 2, 25))},
			{Number: 9, ContentType: "Issue", State: "closed", StateReason: "completed", ClosedAt: timePtr(date(2026, 3, 1))},
			{Number: 12, ContentType: "Issue", State: "closed", StateReason: "not_planned", ClosedAt: timePtr(date(2026, 2, 22))},
			// Older iteration (Feb 3 – Feb 17): 1 closed
			{Number: 5, ContentType: "Issue", State: "closed", StateReason: "completed", ClosedAt: timePtr(date(2026, 2, 10))},
		},
	}

	e, _ := NewEffortEvaluator(p.Config.Effort)
	p.evaluator = e

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	r := p.Result
	if r.Repository != "test/repo" {
		t.Errorf("Repository = %q", r.Repository)
	}
	if r.Unit != "issues" {
		t.Errorf("Unit = %q", r.Unit)
	}

	// Current iteration.
	if r.Current == nil {
		t.Fatal("Current is nil")
	}
	if r.Current.Velocity != 2 {
		t.Errorf("Current.Velocity = %.0f, want 2", r.Current.Velocity)
	}
	if r.Current.ItemsDone != 2 {
		t.Errorf("Current.ItemsDone = %d, want 2", r.Current.ItemsDone)
	}

	// History.
	if len(r.History) != 3 {
		t.Fatalf("History len = %d, want 3", len(r.History))
	}

	// Most recent history: Feb 17 – Mar 3 should have 3 done (not_planned excluded).
	h0 := r.History[0]
	if h0.Velocity != 3 {
		t.Errorf("History[0].Velocity = %.0f, want 3", h0.Velocity)
	}
	if h0.ItemsDone != 3 {
		t.Errorf("History[0].ItemsDone = %d, want 3", h0.ItemsDone)
	}

	// h[1] = Feb 3 – Feb 17 should have 1 (item #5 closed Feb 10).
	h1 := r.History[1]
	if h1.Velocity != 1 {
		t.Errorf("History[1].Velocity = %.0f, want 1", h1.Velocity)
	}

	// h[2] = Jan 20 – Feb 3 should have 0.
	h2 := r.History[2]
	if h2.Velocity != 0 {
		t.Errorf("History[2].Velocity = %.0f, want 0", h2.Velocity)
	}

	// Avg: h[0]=3, h[1]=1, h[2]=0 → avg = 4/3 ≈ 1.33
	expectedAvg := float64(3+1+0) / 3.0
	if diff := r.AvgVelocity - expectedAvg; diff > 0.01 || diff < -0.01 {
		t.Errorf("AvgVelocity = %.2f, want %.2f", r.AvgVelocity, expectedAvg)
	}
}

func TestProcessData_EmptyIteration(t *testing.T) {
	now := date(2026, 3, 10)
	fp, _ := NewFixedPeriod(config.FixedIterationConfig{
		Length: "14d",
		Anchor: "2026-01-06",
	}, now)

	p := &Pipeline{
		Owner: "test", Repo: "repo",
		Config: config.VelocityConfig{
			Unit:      "issues",
			Effort:    config.EffortConfig{Strategy: "count"},
			Iteration: config.IterationConfig{Strategy: "fixed", Count: 1},
		},
		Now:            now,
		IterationCount: 1,
		periods:        fp,
		items:          []model.VelocityItem{}, // no items at all
	}

	e, _ := NewEffortEvaluator(p.Config.Effort)
	p.evaluator = e

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	// Empty iterations should show as zeros, not be omitted.
	if p.Result.Current == nil {
		t.Fatal("Current should exist even with no items")
	}
	if p.Result.Current.Velocity != 0 {
		t.Errorf("Current.Velocity = %.0f, want 0", p.Result.Current.Velocity)
	}
	if len(p.Result.History) != 1 {
		t.Fatalf("History len = %d, want 1", len(p.Result.History))
	}
	if p.Result.History[0].Velocity != 0 {
		t.Errorf("History[0].Velocity = %.0f, want 0", p.Result.History[0].Velocity)
	}
}

func TestProcessData_ShowCurrentOnly(t *testing.T) {
	now := date(2026, 3, 10)
	fp, _ := NewFixedPeriod(config.FixedIterationConfig{
		Length: "14d", Anchor: "2026-01-06",
	}, now)

	p := &Pipeline{
		Owner: "test", Repo: "repo",
		Config: config.VelocityConfig{
			Unit:      "issues",
			Effort:    config.EffortConfig{Strategy: "count"},
			Iteration: config.IterationConfig{Strategy: "fixed", Count: 6},
		},
		Now:            now,
		IterationCount: 6,
		ShowCurrent:    true, // --current flag
		periods:        fp,
		items:          []model.VelocityItem{},
	}
	e, _ := NewEffortEvaluator(p.Config.Effort)
	p.evaluator = e

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	if p.Result.Current == nil {
		t.Fatal("Current should exist")
	}
	if len(p.Result.History) != 0 {
		t.Errorf("History should be empty with --current, got %d", len(p.Result.History))
	}
}

func TestProcessData_ShowHistoryOnly(t *testing.T) {
	now := date(2026, 3, 10)
	fp, _ := NewFixedPeriod(config.FixedIterationConfig{
		Length: "14d", Anchor: "2026-01-06",
	}, now)

	p := &Pipeline{
		Owner: "test", Repo: "repo",
		Config: config.VelocityConfig{
			Unit:      "issues",
			Effort:    config.EffortConfig{Strategy: "count"},
			Iteration: config.IterationConfig{Strategy: "fixed", Count: 2},
		},
		Now:            now,
		IterationCount: 2,
		ShowHistory:    true, // --history flag
		periods:        fp,
		items:          []model.VelocityItem{},
	}
	e, _ := NewEffortEvaluator(p.Config.Effort)
	p.evaluator = e

	if err := p.ProcessData(); err != nil {
		t.Fatalf("ProcessData: %v", err)
	}

	if p.Result.Current != nil {
		t.Fatal("Current should be nil with --history")
	}
	if len(p.Result.History) != 2 {
		t.Errorf("History len = %d, want 2", len(p.Result.History))
	}
}

func TestWriteJSON(t *testing.T) {
	r := model.VelocityResult{
		Repository: "test/repo",
		Unit:       "issues",
		EffortUnit: "items",
		Current: &model.IterationVelocity{
			Name: "Sprint 6", Start: date(2026, 3, 3), End: date(2026, 3, 17),
			Velocity: 5, Committed: 8, CompletionPct: 62.5,
			ItemsDone: 5, ItemsTotal: 8,
		},
		History: []model.IterationVelocity{
			{Name: "Sprint 5", Velocity: 10, Committed: 10, CompletionPct: 100},
		},
		AvgVelocity:   10,
		AvgCompletion: 100,
	}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, r); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	// Verify it's valid JSON.
	var out jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if out.Repository != "test/repo" {
		t.Errorf("repository = %q", out.Repository)
	}
	if out.Current == nil {
		t.Fatal("current is nil in JSON")
	}
	if out.Current.Velocity != 5 {
		t.Errorf("current.velocity = %.0f", out.Current.Velocity)
	}
	if len(out.History) != 1 {
		t.Errorf("history len = %d", len(out.History))
	}
}

func TestWritePretty(t *testing.T) {
	r := model.VelocityResult{
		Repository: "test/repo",
		Unit:       "issues",
		EffortUnit: "items",
		Current: &model.IterationVelocity{
			Name: "Sprint 6", Start: date(2026, 3, 3), End: date(2026, 3, 17),
			Velocity: 5, Committed: 8, CompletionPct: 62.5,
			ItemsDone: 5, ItemsTotal: 8, NotAssessed: 1,
			NotAssessedItems: []int{42},
		},
		History: []model.IterationVelocity{
			{Name: "Sprint 5", Velocity: 10, Committed: 10, CompletionPct: 100, Trend: "▲"},
		},
		AvgVelocity:   10,
		AvgCompletion: 100,
	}

	var buf bytes.Buffer
	if err := WritePretty(&buf, r, true); err != nil {
		t.Fatalf("WritePretty: %v", err)
	}

	output := buf.String()
	for _, want := range []string{"Velocity:", "Sprint 6", "62%", "#42", "Sprint 5", "Avg velocity"} {
		if !strings.Contains(output, want) {
			t.Errorf("pretty output missing %q:\n%s", want, output)
		}
	}
}

func TestWriteMarkdown(t *testing.T) {
	r := model.VelocityResult{
		Repository: "test/repo",
		Unit:       "issues",
		EffortUnit: "pts",
		History: []model.IterationVelocity{
			{Name: "Sprint 5", Start: date(2026, 2, 17), End: date(2026, 3, 3),
				Velocity: 10, Committed: 12, CompletionPct: 83.3, ItemsDone: 5, ItemsTotal: 6},
		},
		AvgVelocity:   10,
		AvgCompletion: 83.3,
	}

	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, r); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}

	output := buf.String()
	for _, want := range []string{"## Velocity:", "Sprint 5", "10.0", "83%", "pts"} {
		if !strings.Contains(output, want) {
			t.Errorf("markdown output missing %q:\n%s", want, output)
		}
	}
}

func TestIsDone(t *testing.T) {
	p := &Pipeline{
		Config: config.VelocityConfig{Unit: "issues"},
	}

	tests := []struct {
		name string
		item model.VelocityItem
		want bool
	}{
		{
			name: "issue completed",
			item: model.VelocityItem{ContentType: "Issue", State: "closed", StateReason: "completed"},
			want: true,
		},
		{
			name: "issue not planned",
			item: model.VelocityItem{ContentType: "Issue", State: "closed", StateReason: "not_planned"},
			want: false,
		},
		{
			name: "issue closed no reason (search API)",
			item: model.VelocityItem{ContentType: "Issue", State: "closed"},
			want: true,
		},
		{
			name: "PR merged",
			item: model.VelocityItem{ContentType: "PullRequest", MergedAt: timePtr(date(2026, 3, 5))},
			want: true,
		},
		{
			name: "PR closed not merged",
			item: model.VelocityItem{ContentType: "PullRequest", State: "closed"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.isDone(tt.item); got != tt.want {
				t.Errorf("isDone() = %v, want %v", got, tt.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time { return &t }
