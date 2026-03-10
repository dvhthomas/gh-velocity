package metrics

import (
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// CycleTime calculates cycle time from a start event to an end event.
// Returns a Metric with nil Duration if either event is nil.
func CycleTime(start, end *model.Event) model.Metric {
	return NewMetric(start, end)
}
