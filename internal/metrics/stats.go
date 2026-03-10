// Package metrics contains pure metric calculation functions.
// No API calls — all inputs are domain types from model/.
package metrics

import (
	"math"
	"slices"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// ComputeStats calculates mean, median, standard deviation, percentiles,
// and IQR-based outlier detection for a slice of durations.
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

	// Sample standard deviation requires N >= 2
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

	// Percentiles and IQR require meaningful sample sizes.
	// P90/P95 need at least 5 data points to be interpretable.
	// IQR outlier detection needs at least 4 (for distinct quartiles).
	if n >= 5 {
		p90 := percentile(sorted, 90)
		p95 := percentile(sorted, 95)
		stats.P90 = &p90
		stats.P95 = &p95
	}

	if n >= 4 {
		q1 := percentile(sorted, 25)
		q3 := percentile(sorted, 75)
		iqr := q3 - q1
		cutoff := q3 + time.Duration(float64(iqr)*1.5)
		stats.OutlierCutoff = &cutoff
		for _, d := range sorted {
			if d > cutoff {
				stats.OutlierCount++
			}
		}
	}

	return stats
}

// IsOutlier returns true if the metric's duration exceeds the IQR outlier cutoff.
func IsOutlier(m model.Metric, stats model.Stats) bool {
	if m.Duration == nil || stats.OutlierCutoff == nil {
		return false
	}
	return *m.Duration > *stats.OutlierCutoff
}

// percentile computes the p-th percentile using nearest-rank method.
// sorted must be sorted ascending. p is in [0, 100].
func percentile(sorted []time.Duration, p int) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	// Nearest-rank: rank = ceil(p/100 * n)
	rank := int(math.Ceil(float64(p) / 100.0 * float64(n)))
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return sorted[rank-1]
}
