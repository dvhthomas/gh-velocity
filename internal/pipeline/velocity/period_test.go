package velocity

import (
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/config"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func TestProjectFieldPeriod_Current(t *testing.T) {
	now := date(2026, 3, 10) // Tuesday, Mar 10
	p := &ProjectFieldPeriod{
		Active: []model.Iteration{
			{ID: "iter-active", Title: "Sprint 6", StartDate: date(2026, 3, 3), Duration: 14, EndDate: date(2026, 3, 17)},
		},
		Completed: []model.Iteration{
			{ID: "iter-5", Title: "Sprint 5", StartDate: date(2026, 2, 17), Duration: 14, EndDate: date(2026, 3, 3)},
		},
		Now: now,
	}

	current, err := p.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if current.Title != "Sprint 6" {
		t.Errorf("Current = %q, want Sprint 6", current.Title)
	}
}

func TestProjectFieldPeriod_CurrentFromCompleted(t *testing.T) {
	// Edge case: current iteration just moved to completed.
	now := date(2026, 3, 3) // exact boundary
	p := &ProjectFieldPeriod{
		Active: []model.Iteration{
			{ID: "iter-6", Title: "Sprint 7", StartDate: date(2026, 3, 17), Duration: 14, EndDate: date(2026, 3, 31)},
		},
		Completed: []model.Iteration{
			{ID: "iter-5", Title: "Sprint 6", StartDate: date(2026, 3, 3), Duration: 14, EndDate: date(2026, 3, 17)},
		},
		Now: now,
	}

	current, err := p.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if current.Title != "Sprint 6" {
		t.Errorf("Current = %q, want Sprint 6", current.Title)
	}
}

func TestProjectFieldPeriod_NoCurrent(t *testing.T) {
	now := date(2026, 6, 1) // Way after all iterations.
	p := &ProjectFieldPeriod{
		Active:    []model.Iteration{},
		Completed: []model.Iteration{},
		Now:       now,
	}
	_, err := p.Current()
	if err == nil {
		t.Fatal("expected error when no iteration spans now")
	}
}

func TestProjectFieldPeriod_Iterations(t *testing.T) {
	p := &ProjectFieldPeriod{
		Completed: []model.Iteration{
			{ID: "1", Title: "Sprint 1", StartDate: date(2026, 1, 6)},
			{ID: "3", Title: "Sprint 3", StartDate: date(2026, 2, 3)},
			{ID: "2", Title: "Sprint 2", StartDate: date(2026, 1, 20)},
		},
	}

	iters, err := p.Iterations(2)
	if err != nil {
		t.Fatalf("Iterations: %v", err)
	}
	if len(iters) != 2 {
		t.Fatalf("got %d iterations, want 2", len(iters))
	}
	// Should be newest first.
	if iters[0].Title != "Sprint 3" {
		t.Errorf("iters[0] = %q, want Sprint 3", iters[0].Title)
	}
	if iters[1].Title != "Sprint 2" {
		t.Errorf("iters[1] = %q, want Sprint 2", iters[1].Title)
	}
}

func TestProjectFieldPeriod_IterationsExceedsCount(t *testing.T) {
	p := &ProjectFieldPeriod{
		Completed: []model.Iteration{
			{ID: "1", Title: "Sprint 1", StartDate: date(2026, 1, 6)},
		},
	}

	iters, err := p.Iterations(5)
	if err != nil {
		t.Fatalf("Iterations: %v", err)
	}
	if len(iters) != 1 {
		t.Fatalf("got %d iterations, want 1 (capped)", len(iters))
	}
}

func TestFixedPeriod_Current(t *testing.T) {
	tests := []struct {
		name      string
		anchor    string
		length    string
		now       time.Time
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "now in first period after anchor",
			anchor:    "2026-01-06",
			length:    "14d",
			now:       date(2026, 1, 10),
			wantStart: date(2026, 1, 6),
			wantEnd:   date(2026, 1, 20),
		},
		{
			name:      "now on anchor exactly",
			anchor:    "2026-01-06",
			length:    "14d",
			now:       date(2026, 1, 6),
			wantStart: date(2026, 1, 6),
			wantEnd:   date(2026, 1, 20),
		},
		{
			name:      "now several periods after anchor",
			anchor:    "2026-01-06",
			length:    "14d",
			now:       date(2026, 3, 10),
			wantStart: date(2026, 3, 3),
			wantEnd:   date(2026, 3, 17),
		},
		{
			name:      "now before anchor",
			anchor:    "2026-03-01",
			length:    "7d",
			now:       date(2026, 2, 25),
			wantStart: date(2026, 2, 22),
			wantEnd:   date(2026, 3, 1),
		},
		{
			name:      "weekly iterations",
			anchor:    "2026-01-05",
			length:    "1w",
			now:       date(2026, 1, 15),
			wantStart: date(2026, 1, 12),
			wantEnd:   date(2026, 1, 19),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp, err := NewFixedPeriod(config.FixedIterationConfig{
				Length: tt.length,
				Anchor: tt.anchor,
			}, tt.now)
			if err != nil {
				t.Fatalf("NewFixedPeriod: %v", err)
			}

			current, err := fp.Current()
			if err != nil {
				t.Fatalf("Current: %v", err)
			}

			if !current.StartDate.Equal(tt.wantStart) {
				t.Errorf("Start = %s, want %s", current.StartDate.Format(time.DateOnly), tt.wantStart.Format(time.DateOnly))
			}
			if !current.EndDate.Equal(tt.wantEnd) {
				t.Errorf("End = %s, want %s", current.EndDate.Format(time.DateOnly), tt.wantEnd.Format(time.DateOnly))
			}
		})
	}
}

func TestFixedPeriod_Iterations(t *testing.T) {
	fp, err := NewFixedPeriod(config.FixedIterationConfig{
		Length: "14d",
		Anchor: "2026-01-06",
	}, date(2026, 3, 10))
	if err != nil {
		t.Fatalf("NewFixedPeriod: %v", err)
	}

	iters, err := fp.Iterations(3)
	if err != nil {
		t.Fatalf("Iterations: %v", err)
	}

	if len(iters) != 3 {
		t.Fatalf("got %d iterations, want 3", len(iters))
	}

	// Current is Mar 3 – Mar 17, so iterations should be the 3 before that.
	expected := []struct {
		start time.Time
		end   time.Time
	}{
		{date(2026, 2, 17), date(2026, 3, 3)}, // most recent completed
		{date(2026, 2, 3), date(2026, 2, 17)},
		{date(2026, 1, 20), date(2026, 2, 3)},
	}

	for i, want := range expected {
		got := iters[i]
		if !got.StartDate.Equal(want.start) {
			t.Errorf("iters[%d].Start = %s, want %s", i, got.StartDate.Format(time.DateOnly), want.start.Format(time.DateOnly))
		}
		if !got.EndDate.Equal(want.end) {
			t.Errorf("iters[%d].End = %s, want %s", i, got.EndDate.Format(time.DateOnly), want.end.Format(time.DateOnly))
		}
	}
}

func TestFormatDateRange(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  string
	}{
		{
			name:  "same month",
			start: date(2026, 3, 4),
			end:   date(2026, 3, 17),
			want:  "Mar 4 – 17",
		},
		{
			name:  "different months",
			start: date(2026, 2, 17),
			end:   date(2026, 3, 2),
			want:  "Feb 17 – Mar 2",
		},
		{
			name:  "different years",
			start: date(2025, 12, 22),
			end:   date(2026, 1, 4),
			want:  "Dec 22, 2025 – Jan 4, 2026",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDateRange(tt.start, tt.end)
			if got != tt.want {
				t.Errorf("formatDateRange = %q, want %q", got, tt.want)
			}
		})
	}
}
