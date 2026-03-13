package velocity

import (
	"fmt"
	"sort"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/config"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// PeriodStrategy resolves iteration boundaries.
type PeriodStrategy interface {
	// Current returns the iteration spanning now.
	Current() (*model.Iteration, error)
	// Iterations returns the last count completed iterations, ordered newest first.
	Iterations(count int) ([]model.Iteration, error)
}

// ProjectFieldPeriod resolves iterations from a ProjectV2 Iteration field.
type ProjectFieldPeriod struct {
	Active    []model.Iteration // from configuration.iterations (active/upcoming)
	Completed []model.Iteration // from configuration.completedIterations
	Now       time.Time
}

func (p *ProjectFieldPeriod) Current() (*model.Iteration, error) {
	// Check active iterations first (most likely to be current).
	for i := range p.Active {
		it := &p.Active[i]
		if !p.Now.Before(it.StartDate) && p.Now.Before(it.EndDate) {
			return it, nil
		}
	}
	// Check completed iterations (edge case: current iteration just completed).
	for i := range p.Completed {
		it := &p.Completed[i]
		if !p.Now.Before(it.StartDate) && p.Now.Before(it.EndDate) {
			return it, nil
		}
	}
	return nil, fmt.Errorf("no current iteration found spanning %s", p.Now.Format(time.DateOnly))
}

func (p *ProjectFieldPeriod) Iterations(count int) ([]model.Iteration, error) {
	// Sort completed iterations by start date descending (newest first).
	sorted := make([]model.Iteration, len(p.Completed))
	copy(sorted, p.Completed)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartDate.After(sorted[j].StartDate)
	})

	if count > len(sorted) {
		count = len(sorted)
	}
	return sorted[:count], nil
}

// FixedPeriod computes iteration boundaries from a fixed length and anchor date.
type FixedPeriod struct {
	Length time.Duration
	Anchor time.Time
	Now    time.Time
}

// NewFixedPeriod creates a FixedPeriod from config.
func NewFixedPeriod(cfg config.FixedIterationConfig, now time.Time) (*FixedPeriod, error) {
	length, err := config.ParseFixedLength(cfg.Length)
	if err != nil {
		return nil, err
	}
	anchor, err := time.Parse(time.DateOnly, cfg.Anchor)
	if err != nil {
		return nil, fmt.Errorf("parse anchor date: %w", err)
	}
	return &FixedPeriod{Length: length, Anchor: anchor, Now: now}, nil
}

func (p *FixedPeriod) Current() (*model.Iteration, error) {
	it := p.iterationAt(p.Now)
	return &it, nil
}

func (p *FixedPeriod) Iterations(count int) ([]model.Iteration, error) {
	current := p.iterationAt(p.Now)
	result := make([]model.Iteration, 0, count)

	// Walk backward from the iteration before current.
	t := current.StartDate.Add(-p.Length)
	for i := 0; i < count; i++ {
		it := p.iterationAt(t)
		result = append(result, it)
		t = it.StartDate.Add(-p.Length)
	}
	return result, nil
}

// iterationAt returns the iteration containing time t.
func (p *FixedPeriod) iterationAt(t time.Time) model.Iteration {
	days := int(p.Length.Hours() / 24)

	// Compute how many periods from anchor to t.
	diff := t.Sub(p.Anchor)
	periods := int(diff / p.Length)

	// If t is before anchor, adjust to get the correct period start.
	if diff < 0 {
		periods-- // one more period back
	}

	start := p.Anchor.Add(time.Duration(periods) * p.Length)
	end := start.Add(p.Length)

	return model.Iteration{
		Title:     formatDateRange(start, end.Add(-24*time.Hour)), // end is exclusive, show last day
		StartDate: start,
		Duration:  days,
		EndDate:   end,
	}
}

// formatDateRange formats a date range like "Mar 4 – Mar 17".
func formatDateRange(start, end time.Time) string {
	if start.Year() != end.Year() {
		return fmt.Sprintf("%s %d, %d – %s %d, %d",
			start.Month().String()[:3], start.Day(), start.Year(),
			end.Month().String()[:3], end.Day(), end.Year())
	}
	if start.Month() != end.Month() {
		return fmt.Sprintf("%s %d – %s %d",
			start.Month().String()[:3], start.Day(),
			end.Month().String()[:3], end.Day())
	}
	return fmt.Sprintf("%s %d – %d",
		start.Month().String()[:3], start.Day(), end.Day())
}
