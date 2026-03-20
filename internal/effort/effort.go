// Package effort provides shared effort evaluation for work items.
// It is used by the velocity pipeline and the WIP detail report.
package effort

import (
	"fmt"
	"strings"

	"github.com/dvhthomas/gh-velocity/internal/classify"
	"github.com/dvhthomas/gh-velocity/internal/config"
)

// Item is a generic work item that effort evaluators operate on.
type Item struct {
	Labels    []string
	IssueType string
	Title     string
	Fields    map[string]string
	Effort    *float64 // nil when no numeric data available
}

// Evaluator assigns an effort value to a work item.
type Evaluator interface {
	// Evaluate returns the effort value and whether the item was assessed.
	// If assessed is false, the item is "not assessed" (no effort assigned).
	Evaluate(item Item) (effort float64, assessed bool)
}

// CountEvaluator assigns effort = 1 to every item.
type CountEvaluator struct{}

func (CountEvaluator) Evaluate(_ Item) (float64, bool) {
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

func (e *AttributeEvaluator) Evaluate(item Item) (float64, bool) {
	input := classify.Input{
		Labels:    item.Labels,
		IssueType: item.IssueType,
		Title:     item.Title,
		Fields:    item.Fields,
	}
	for _, m := range e.matchers {
		if m.matcher.Matches(input) {
			return m.value, true
		}
	}
	return 0, false
}

// NumericEvaluator reads effort from the item's Effort field (project Number field).
// nil = not assessed, 0 = valid zero, negative = not assessed.
type NumericEvaluator struct{}

func (NumericEvaluator) Evaluate(item Item) (float64, bool) {
	if item.Effort == nil {
		return 0, false
	}
	v := *item.Effort
	if v < 0 {
		return 0, false
	}
	return v, true
}

// NewEvaluator creates an Evaluator from the config.
func NewEvaluator(cfg config.EffortConfig) (Evaluator, error) {
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

// HasFieldMatchers returns true if any effort attribute matcher uses the "field:" prefix.
func HasFieldMatchers(cfg config.EffortConfig) bool {
	if cfg.Strategy != "attribute" {
		return false
	}
	for _, m := range cfg.Attribute {
		if strings.HasPrefix(m.Query, "field:") {
			return true
		}
	}
	return false
}

// ExtractFieldMatcherNames returns the unique SingleSelect field names used in
// "field:Name/Value" effort matchers.
func ExtractFieldMatcherNames(cfg config.EffortConfig) []string {
	if cfg.Strategy != "attribute" {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, m := range cfg.Attribute {
		if !strings.HasPrefix(m.Query, "field:") {
			continue
		}
		rest := strings.TrimPrefix(m.Query, "field:")
		name, _, ok := strings.Cut(rest, "/")
		if !ok || name == "" {
			continue
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}
