package metrics

import (
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// Staleness thresholds (hardcoded — add flags only if users need overrides).
const (
	stalenessAgingDays = 3
	stalenssStaleDays  = 7
)

// ComputeStaleness classifies the staleness of an item based on its last
// activity time relative to now.
func ComputeStaleness(updatedAt time.Time, now time.Time) model.StalenessLevel {
	days := now.Sub(updatedAt).Hours() / 24
	switch {
	case days > float64(stalenssStaleDays):
		return model.StalenessStale
	case days > float64(stalenessAgingDays):
		return model.StalenessAging
	default:
		return model.StalenessActive
	}
}
