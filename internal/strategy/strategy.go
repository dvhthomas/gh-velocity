// Package strategy implements linking strategies for discovering which
// issues and PRs belong to a release. Multiple strategies run and their
// results are merged (union) with priority-based deduplication.
package strategy

import (
	"context"
	"fmt"
	"time"

	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// Strategy discovers issues and PRs belonging to a release.
type Strategy interface {
	// Name returns the strategy identifier (e.g., "pr-link", "commit-ref", "changelog").
	Name() string
	// Discover finds issues/PRs for the release described by input.
	Discover(ctx context.Context, input DiscoverInput) ([]model.DiscoveredItem, error)
}

// DiscoverInput provides the data each strategy needs.
type DiscoverInput struct {
	Owner       string
	Repo        string
	Tag         string
	PreviousTag string
	TagDate     time.Time // creation date of the target tag
	PrevTagDate time.Time // creation date of the previous tag (zero if none)
	Commits     []model.Commit
	Release     *model.Release // GitHub release (for changelog body)
	Client      *gh.Client

	// CommitRefPatterns controls which commit-ref patterns to match.
	// Default: ["closes"]. Add "refs" for bare #N matching.
	CommitRefPatterns []string
}

// DefaultMaxWindowDays is the default time window limit between tags.
const DefaultMaxWindowDays = 31

// HardMaxWindowDays is the absolute maximum configurable time window.
const HardMaxWindowDays = 90

// Runner executes all strategies and merges results.
type Runner struct {
	strategies    []Strategy
	maxWindowDays int
}

// NewRunner creates a Runner with the given strategies.
// If maxWindowDays is 0, DefaultMaxWindowDays is used.
func NewRunner(maxWindowDays int, strategies ...Strategy) *Runner {
	if maxWindowDays <= 0 {
		maxWindowDays = DefaultMaxWindowDays
	}
	if maxWindowDays > HardMaxWindowDays {
		maxWindowDays = HardMaxWindowDays
	}
	return &Runner{
		strategies:    strategies,
		maxWindowDays: maxWindowDays,
	}
}

// Run executes all strategies and returns a ScopeResult with per-strategy
// and merged results.
func (r *Runner) Run(ctx context.Context, input DiscoverInput) (*model.ScopeResult, []string, error) {
	var warnings []string

	// Enforce time window guardrails
	if !input.PrevTagDate.IsZero() && !input.TagDate.IsZero() {
		windowDays := int(input.TagDate.Sub(input.PrevTagDate).Hours() / 24)
		if windowDays > r.maxWindowDays {
			return nil, nil, fmt.Errorf(
				"time window between tags is %d days (max %d). Use --since with a closer tag or increase max_window_days in config (max %d)",
				windowDays, r.maxWindowDays, HardMaxWindowDays,
			)
		}
	}

	var stratResults []model.StrategyResult

	for _, s := range r.strategies {
		items, err := s.Discover(ctx, input)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("strategy %s: %v", s.Name(), err))
			continue
		}
		stratResults = append(stratResults, model.StrategyResult{
			Name:  s.Name(),
			Items: items,
		})
	}

	merged := Merge(stratResults)

	result := &model.ScopeResult{
		Tag:         input.Tag,
		PreviousTag: input.PreviousTag,
		Strategies:  stratResults,
		Merged:      merged,
	}

	return result, warnings, nil
}
