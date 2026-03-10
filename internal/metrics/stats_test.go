package metrics

import (
	"testing"
	"time"
)

func TestComputeStats_Empty(t *testing.T) {
	stats := ComputeStats(nil)
	if stats.Count != 0 {
		t.Errorf("expected count 0, got %d", stats.Count)
	}
	if stats.Mean != nil {
		t.Error("expected nil mean for empty input")
	}
	if stats.Median != nil {
		t.Error("expected nil median for empty input")
	}
	if stats.StdDev != nil {
		t.Error("expected nil stddev for empty input")
	}
}

func TestComputeStats_Single(t *testing.T) {
	d := 5 * time.Hour
	stats := ComputeStats([]time.Duration{d})

	if stats.Count != 1 {
		t.Errorf("expected count 1, got %d", stats.Count)
	}
	if *stats.Mean != d {
		t.Errorf("expected mean %v, got %v", d, *stats.Mean)
	}
	if *stats.Median != d {
		t.Errorf("expected median %v, got %v", d, *stats.Median)
	}
	if stats.StdDev != nil {
		t.Error("expected nil stddev for N=1")
	}
}

func TestComputeStats_Two(t *testing.T) {
	durations := []time.Duration{2 * time.Hour, 4 * time.Hour}
	stats := ComputeStats(durations)

	if stats.Count != 2 {
		t.Errorf("expected count 2, got %d", stats.Count)
	}

	expectedMean := 3 * time.Hour
	if *stats.Mean != expectedMean {
		t.Errorf("expected mean %v, got %v", expectedMean, *stats.Mean)
	}

	expectedMedian := 3 * time.Hour
	if *stats.Median != expectedMedian {
		t.Errorf("expected median %v, got %v", expectedMedian, *stats.Median)
	}

	if stats.StdDev == nil {
		t.Fatal("expected non-nil stddev for N=2")
	}
}

func TestComputeStats_Odd(t *testing.T) {
	durations := []time.Duration{
		1 * time.Hour,
		3 * time.Hour,
		5 * time.Hour,
	}
	stats := ComputeStats(durations)

	if stats.Count != 3 {
		t.Errorf("expected count 3, got %d", stats.Count)
	}
	if *stats.Median != 3*time.Hour {
		t.Errorf("expected median 3h, got %v", *stats.Median)
	}
}

func TestComputeStats_Unsorted(t *testing.T) {
	durations := []time.Duration{
		5 * time.Hour,
		1 * time.Hour,
		3 * time.Hour,
	}
	stats := ComputeStats(durations)

	if *stats.Median != 3*time.Hour {
		t.Errorf("expected median 3h from unsorted input, got %v", *stats.Median)
	}
}

func TestComputeStats_Large(t *testing.T) {
	durations := make([]time.Duration, 100)
	for i := range durations {
		durations[i] = time.Duration(i+1) * time.Hour
	}
	stats := ComputeStats(durations)

	if stats.Count != 100 {
		t.Errorf("expected count 100, got %d", stats.Count)
	}
	if stats.StdDev == nil {
		t.Error("expected non-nil stddev for N=100")
	}
}

func TestComputeStats_Percentiles(t *testing.T) {
	// 10 values: 1h, 2h, ..., 10h
	durations := make([]time.Duration, 10)
	for i := range durations {
		durations[i] = time.Duration(i+1) * time.Hour
	}
	stats := ComputeStats(durations)

	if stats.P90 == nil {
		t.Fatal("expected non-nil P90")
	}
	// P90 nearest-rank: ceil(0.9 * 10) = 9 → 9h
	if *stats.P90 != 9*time.Hour {
		t.Errorf("expected P90 = 9h, got %v", *stats.P90)
	}
	// P95 nearest-rank: ceil(0.95 * 10) = 10 → 10h
	if *stats.P95 != 10*time.Hour {
		t.Errorf("expected P95 = 10h, got %v", *stats.P95)
	}
}

func TestComputeStats_OutlierDetection(t *testing.T) {
	// 9 normal values + 1 extreme outlier
	durations := []time.Duration{
		1 * time.Hour,
		2 * time.Hour,
		3 * time.Hour,
		4 * time.Hour,
		5 * time.Hour,
		6 * time.Hour,
		7 * time.Hour,
		8 * time.Hour,
		9 * time.Hour,
		100 * time.Hour, // extreme outlier
	}
	stats := ComputeStats(durations)

	if stats.OutlierCutoff == nil {
		t.Fatal("expected non-nil outlier cutoff")
	}
	if stats.OutlierCount != 1 {
		t.Errorf("expected 1 outlier, got %d", stats.OutlierCount)
	}
}

func TestComputeStats_NoOutliers(t *testing.T) {
	// Tight distribution — no outliers
	durations := []time.Duration{
		10 * time.Hour,
		11 * time.Hour,
		12 * time.Hour,
		13 * time.Hour,
		14 * time.Hour,
	}
	stats := ComputeStats(durations)

	if stats.OutlierCount != 0 {
		t.Errorf("expected 0 outliers, got %d", stats.OutlierCount)
	}
}

func TestComputeStats_SmallN_NoPercentiles(t *testing.T) {
	// N=1: no percentiles, no outliers
	stats := ComputeStats([]time.Duration{5 * time.Hour})
	if stats.P90 != nil {
		t.Error("expected nil P90 for N=1")
	}
	if stats.OutlierCutoff != nil {
		t.Error("expected nil outlier cutoff for N=1")
	}

	// N=3: stddev yes, but no percentiles or outliers
	stats = ComputeStats([]time.Duration{1 * time.Hour, 2 * time.Hour, 3 * time.Hour})
	if stats.StdDev == nil {
		t.Error("expected non-nil stddev for N=3")
	}
	if stats.P90 != nil {
		t.Error("expected nil P90 for N=3 (need >= 5)")
	}
	if stats.OutlierCutoff != nil {
		t.Error("expected nil outlier cutoff for N=3 (need >= 4)")
	}

	// N=4: outliers yes, but no percentiles
	stats = ComputeStats([]time.Duration{1 * time.Hour, 2 * time.Hour, 3 * time.Hour, 4 * time.Hour})
	if stats.OutlierCutoff == nil {
		t.Error("expected non-nil outlier cutoff for N=4")
	}
	if stats.P90 != nil {
		t.Error("expected nil P90 for N=4 (need >= 5)")
	}
}

func TestIsOutlier(t *testing.T) {
	durations := []time.Duration{
		1 * time.Hour, 2 * time.Hour, 3 * time.Hour,
		4 * time.Hour, 5 * time.Hour, 100 * time.Hour,
	}
	stats := ComputeStats(durations)

	normal := 3 * time.Hour
	if IsOutlier(&normal, stats) {
		t.Error("3h should not be an outlier")
	}

	extreme := 100 * time.Hour
	if !IsOutlier(&extreme, stats) {
		t.Error("100h should be an outlier")
	}

	if IsOutlier(nil, stats) {
		t.Error("nil duration should not be an outlier")
	}
}
