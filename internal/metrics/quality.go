package metrics

import (
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// IsHotfix returns true if the release is a patch release within the hotfix window.
func IsHotfix(current, previous model.Release, hotfixWindowHours float64) bool {
	if previous.TagName == "" {
		return false
	}
	window := time.Duration(hotfixWindowHours * float64(time.Hour))
	return current.CreatedAt.Sub(previous.CreatedAt) <= window
}
