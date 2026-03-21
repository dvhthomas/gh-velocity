package format

import (
	"cmp"
	"slices"
)

// Flag constants for item annotations in detail tables.
// These unify the previously per-command emoji/text labels into
// a shared vocabulary used across all commands.
const (
	FlagOutlier = "outlier" // 🚩
	FlagNoise   = "noise"   // 🤖
	FlagHotfix  = "hotfix"  // ⚡
	FlagBug     = "bug"     // 🐛
	FlagStale   = "stale"   // ⏳
	FlagAging   = "aging"   // 🟡
)

var flagEmojis = map[string]string{
	FlagOutlier: "🚩",
	FlagNoise:   "🤖",
	FlagHotfix:  "⚡",
	FlagBug:     "🐛",
	FlagStale:   "⏳",
	FlagAging:   "🟡",
}

// FlagEmoji returns the emoji for a flag constant.
// Returns an empty string for unknown flags.
func FlagEmoji(flag string) string {
	return flagEmojis[flag]
}

// Sort direction constants.
const (
	Desc = "desc"
	Asc  = "asc"
)

// Sorted wraps a slice of items with sort metadata. The sort field
// and direction travel with the data so that pretty headers can show
// an arrow and JSON output can include the sort contract.
type Sorted[T any] struct {
	Items     []T
	Field     string // e.g. "lead_time", "cycle_time", "age"
	Direction string // Desc or Asc
}

// SortBy returns a Sorted collection, copying and sorting items by key.
// Nil keys sort to the end regardless of direction.
func SortBy[T any, K cmp.Ordered](items []T, field string, direction string, key func(T) *K) Sorted[T] {
	sorted := make([]T, len(items))
	copy(sorted, items)

	slices.SortFunc(sorted, func(a, b T) int {
		ka, kb := key(a), key(b)
		if ka == nil && kb == nil {
			return 0
		}
		if ka == nil {
			return 1
		}
		if kb == nil {
			return -1
		}
		if direction == Asc {
			return cmp.Compare(*ka, *kb)
		}
		return cmp.Compare(*kb, *ka)
	})

	return Sorted[T]{Items: sorted, Field: field, Direction: direction}
}

// Header returns a column header name with a sort direction arrow
// if headerField matches the sort field.
func (s Sorted[T]) Header(headerField, displayName string) string {
	if s.Field != headerField {
		return displayName
	}
	switch s.Direction {
	case Desc:
		return displayName + " ↓"
	case Asc:
		return displayName + " ↑"
	default:
		return displayName
	}
}

// JSONSort returns the sort metadata for JSON output.
func (s Sorted[T]) JSONSort() JSONSort {
	return JSONSort{Field: s.Field, Direction: s.Direction}
}

