package pipeline

import "github.com/dvhthomas/gh-velocity/internal/model"

// BulkItem holds a single item's metric result for bulk duration-based commands.
// Used by cycletime and leadtime, which have identical data shapes.
type BulkItem struct {
	Issue  model.Issue
	Metric model.Metric
}
