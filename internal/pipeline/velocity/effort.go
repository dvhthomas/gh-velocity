package velocity

import (
	"github.com/dvhthomas/gh-velocity/internal/config"
	"github.com/dvhthomas/gh-velocity/internal/effort"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// MaxBoardItems is the upper bound on project board items fetched.
// Boards exceeding this limit are truncated with a warning.
const MaxBoardItems = 2000

// EffortEvaluator assigns an effort value to a work item.
type EffortEvaluator interface {
	// Evaluate returns the effort value and whether the item was assessed.
	// If assessed is false, the item is "not assessed" (no effort assigned).
	Evaluate(item model.VelocityItem) (effort float64, assessed bool)
}

// velocityEvaluator adapts effort.Evaluator to work with model.VelocityItem.
type velocityEvaluator struct {
	inner effort.Evaluator
}

func (v *velocityEvaluator) Evaluate(item model.VelocityItem) (float64, bool) {
	ei := effort.Item{
		Labels:    item.Labels,
		IssueType: item.IssueType,
		Title:     item.Title,
		Fields:    item.Fields,
		Effort:    item.Effort,
	}
	val, ok := v.inner.Evaluate(ei)
	if !ok && item.Effort != nil && *item.Effort < 0 {
		log.Warn("item #%d has negative effort value %.1f, treating as not assessed", item.Number, *item.Effort)
	}
	return val, ok
}

// NewEffortEvaluator creates an EffortEvaluator from the config.
func NewEffortEvaluator(cfg config.EffortConfig) (EffortEvaluator, error) {
	inner, err := effort.NewEvaluator(cfg)
	if err != nil {
		return nil, err
	}
	return &velocityEvaluator{inner: inner}, nil
}

// HasFieldMatchers returns true if any effort attribute matcher uses the "field:" prefix.
func HasFieldMatchers(cfg config.EffortConfig) bool {
	return effort.HasFieldMatchers(cfg)
}

// ExtractFieldMatcherNames returns the unique SingleSelect field names used in
// "field:Name/Value" effort matchers.
func ExtractFieldMatcherNames(cfg config.EffortConfig) []string {
	return effort.ExtractFieldMatcherNames(cfg)
}
