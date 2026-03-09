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
