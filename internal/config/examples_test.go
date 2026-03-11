package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bitsbyme/gh-velocity/internal/config"
)

func TestExampleConfigsParse(t *testing.T) {
	files, err := filepath.Glob("../../docs/examples/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no example configs found")
	}
	for _, f := range files {
		name := filepath.Base(f)
		if name == "velocity-report.yml" {
			continue // GitHub Actions workflow, not a gh-velocity config
		}
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			cfg, err := config.Parse(data)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if cfg.CycleTime.Strategy == "" {
				t.Error("expected non-empty cycle_time.strategy")
			}
			if len(cfg.Quality.Categories) == 0 {
				t.Error("expected at least one category")
			}
		})
	}
}
