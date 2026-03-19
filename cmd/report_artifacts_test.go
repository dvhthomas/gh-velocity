package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/pipeline/leadtime"
	"github.com/dvhthomas/gh-velocity/internal/pipeline/throughput"
)

func TestWriteReportArtifacts_CreatesPerSectionFiles(t *testing.T) {
	dir := t.TempDir()
	deps := &Deps{Output: OutputConfig{Results: []format.Format{format.Markdown}}}

	now := time.Now()
	since := now.Add(-7 * 24 * time.Hour)
	dur := 48 * time.Hour

	result := model.StatsResult{
		Repository: "test/repo",
		Since:      since,
		Until:      now,
		LeadTime:   &model.Stats{Count: 5, Mean: &dur, Median: &dur},
		Throughput: &model.StatsThroughput{IssuesClosed: 10, PRsMerged: 5},
	}

	leadPipeline := &leadtime.BulkPipeline{
		Owner: "test", Repo: "repo",
		Since: since, Until: now,
		Items: []leadtime.BulkItem{
			{
				Issue:  model.Issue{Number: 1, Title: "Test issue", CreatedAt: since, ClosedAt: &now},
				Metric: model.Metric{Duration: &dur},
			},
		},
		Stats: *result.LeadTime,
	}

	throughputPipeline := &throughput.Pipeline{
		Owner: "test", Repo: "repo",
		Since: since, Until: now,
		Result: model.ThroughputResult{
			Repository:   "test/repo",
			Since:        since,
			Until:        now,
			IssuesClosed: 10,
			PRsMerged:    5,
		},
	}

	sections := []artifactSection{
		leadTimeArtifact(leadPipeline),
		throughputArtifact(throughputPipeline),
	}

	if err := writeReportArtifacts(deps, dir, sections); err != nil {
		t.Fatalf("writeReportArtifacts failed: %v", err)
	}

	// Verify expected files exist.
	expectedFiles := []string{
		"flow-lead-time.json",
		"flow-lead-time.md",
		"flow-throughput.json",
		"flow-throughput.md",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(dir, f)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected file %s to exist, got error: %v", f, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("expected file %s to be non-empty", f)
		}
	}
}

func TestWriteReportArtifacts_NoSections(t *testing.T) {
	dir := t.TempDir()
	deps := &Deps{Output: OutputConfig{Results: []format.Format{format.Markdown}}}

	// No sections — should succeed without writing any files.
	if err := writeReportArtifacts(deps, dir, nil); err != nil {
		t.Fatalf("writeReportArtifacts failed: %v", err)
	}

	// Per-section files should not exist when there are no sections.
	for _, f := range []string{"flow-lead-time.json", "flow-lead-time.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("expected %s to NOT exist", f)
		}
	}

	// No per-section files should exist.
	for _, f := range []string{"flow-lead-time.json", "flow-throughput.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("did not expect %s to exist when no sections provided", f)
		}
	}
}
