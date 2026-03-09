// Package metrics contains pure metric calculation functions.
// No API calls — all inputs are domain types from model/.
package metrics

import (
	"math"
	"slices"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// ComputeStats calculates mean, median, and sample standard deviation for a slice of durations.
func ComputeStats(durations []time.Duration) model.Stats {
	n := len(durations)
	if n == 0 {
		return model.Stats{Count: 0}
	}

	sorted := make([]time.Duration, n)
	copy(sorted, durations)
	slices.Sort(sorted)

	// Mean
	var sum float64
	for _, d := range sorted {
		sum += float64(d)
	}
	meanF := sum / float64(n)
	mean := time.Duration(meanF)

	// Median
	var median time.Duration
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	} else {
		median = sorted[n/2]
	}

	stats := model.Stats{
		Count:  n,
		Mean:   &mean,
		Median: &median,
	}

	// Sample standard deviation (N-1 denominator), only for N >= 2
	if n >= 2 {
		var sumSq float64
		for _, d := range sorted {
			diff := float64(d) - meanF
			sumSq += diff * diff
		}
		sdF := math.Sqrt(sumSq / float64(n-1))
		sd := time.Duration(sdF)
		stats.StdDev = &sd
	}

	return stats
}
