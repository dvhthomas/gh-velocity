package velocity

import (
	"fmt"

	"github.com/bitsbyme/gh-velocity/internal/classify"
	"github.com/bitsbyme/gh-velocity/internal/config"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// EffortEvaluator assigns an effort value to a work item.
type EffortEvaluator interface {
	// Evaluate returns the effort value and whether the item was assessed.
	// If assessed is false, the item is "not assessed" (no effort assigned).
	Evaluate(item model.VelocityItem) (effort float64, assessed bool)
}

// CountEvaluator assigns effort = 1 to every item.
type CountEvaluator struct{}

func (CountEvaluator) Evaluate(_ model.VelocityItem) (float64, bool) {
	return 1, true
}

// compiledMatcher pairs a parsed classify.Matcher with its effort value.
type compiledMatcher struct {
	matcher classify.Matcher
	value   float64
}

// AttributeEvaluator evaluates effort using classify.Matcher rules.
// First match wins (config order). Unmatched items are not assessed.
type AttributeEvaluator struct {
	matchers []compiledMatcher
}

func (e *AttributeEvaluator) Evaluate(item model.VelocityItem) (float64, bool) {
	input := classify.Input{
		Labels:    item.Labels,
		IssueType: item.IssueType,
		Title:     item.Title,
	}
	for _, m := range e.matchers {
		if m.matcher.Matches(input) {
			return m.value, true
		}
	}
	return 0, false
}

// NumericEvaluator reads effort from the item's Effort field (project Number field).
// nil = not assessed, 0 = valid zero, negative = not assessed + warning.
type NumericEvaluator struct{}

func (NumericEvaluator) Evaluate(item model.VelocityItem) (float64, bool) {
	if item.Effort == nil {
		return 0, false
	}
	v := *item.Effort
	if v < 0 {
		log.Warn("item #%d has negative effort value %.1f, treating as not assessed", item.Number, v)
		return 0, false
	}
	return v, true
}

// NewEffortEvaluator creates an EffortEvaluator from the config.
func NewEffortEvaluator(cfg config.EffortConfig) (EffortEvaluator, error) {
	switch cfg.Strategy {
	case "count":
		return CountEvaluator{}, nil
	case "attribute":
		matchers := make([]compiledMatcher, len(cfg.Attribute))
		for i, m := range cfg.Attribute {
			parsed, err := classify.ParseMatcher(m.Query)
			if err != nil {
				return nil, fmt.Errorf("effort matcher[%d]: %w", i, err)
			}
			matchers[i] = compiledMatcher{matcher: parsed, value: m.Value}
		}
		return &AttributeEvaluator{matchers: matchers}, nil
	case "numeric":
		return NumericEvaluator{}, nil
	default:
		return nil, fmt.Errorf("unknown effort strategy %q", cfg.Strategy)
	}
}
