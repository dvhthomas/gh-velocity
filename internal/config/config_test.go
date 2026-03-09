package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_MissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/.gh-velocity.yml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg.Workflow != DefaultWorkflow {
		t.Errorf("expected default workflow %q, got %q", DefaultWorkflow, cfg.Workflow)
	}
	if cfg.Quality.HotfixWindowHours != DefaultHotfixWindowHours {
		t.Errorf("expected default hotfix window %v, got %v", DefaultHotfixWindowHours, cfg.Quality.HotfixWindowHours)
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gh-velocity.yml")

	content := `
workflow: local
quality:
  bug_labels: ["bug", "defect"]
  feature_labels: ["feature"]
  hotfix_window_hours: 48
project:
  id: "PVT_abc123"
  status_field_id: "PVTSSF_def456"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Workflow != "local" {
		t.Errorf("expected workflow 'local', got %q", cfg.Workflow)
	}
	if len(cfg.Quality.BugLabels) != 2 {
		t.Errorf("expected 2 bug labels, got %d", len(cfg.Quality.BugLabels))
	}
	if cfg.Quality.HotfixWindowHours != 48 {
		t.Errorf("expected hotfix_window_hours 48, got %v", cfg.Quality.HotfixWindowHours)
	}
	if cfg.Project.ID != "PVT_abc123" {
		t.Errorf("expected project.id 'PVT_abc123', got %q", cfg.Project.ID)
	}
}

func TestLoad_InvalidWorkflow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gh-velocity.yml")

	content := `workflow: auto`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid workflow")
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gh-velocity.yml")

	content := `workflow: [invalid yaml`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestLoad_OversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gh-velocity.yml")

	data := make([]byte, MaxConfigFileSize+1)
	for i := range data {
		data[i] = '#'
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
}

func TestLoad_NaNHotfixWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gh-velocity.yml")

	content := `quality:
  hotfix_window_hours: .nan
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for NaN hotfix_window_hours")
	}
}

func TestLoad_NegativeHotfixWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gh-velocity.yml")

	content := `quality:
  hotfix_window_hours: -10
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for negative hotfix_window_hours")
	}
}

func TestLoad_PartialConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gh-velocity.yml")

	content := `quality:
  bug_labels: ["bug"]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Partial config should retain defaults for unset fields
	if cfg.Workflow != DefaultWorkflow {
		t.Errorf("expected default workflow, got %q", cfg.Workflow)
	}
}

func TestLoad_UnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gh-velocity.yml")

	content := `
workflow: pr
future_field: something
nested:
  unknown: true
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Capture warnings.
	var warnings []string
	origWarn := WarnFunc
	WarnFunc = func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}
	defer func() { WarnFunc = origWarn }()

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unknown keys should not cause errors: %v", err)
	}
	if cfg.Workflow != "pr" {
		t.Errorf("expected workflow 'pr', got %q", cfg.Workflow)
	}

	// Should have warnings for "future_field" and "nested".
	foundFutureField := false
	foundNested := false
	for _, w := range warnings {
		if strings.Contains(w, "future_field") {
			foundFutureField = true
		}
		if strings.Contains(w, "nested") {
			foundNested = true
		}
	}
	if !foundFutureField {
		t.Error("expected warning about unknown key 'future_field'")
	}
	if !foundNested {
		t.Error("expected warning about unknown key 'nested'")
	}
}

func TestLoad_Validation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string // substring expected in error; empty means no error
	}{
		{
			name:    "hotfix_window_hours zero",
			yaml:    "quality:\n  hotfix_window_hours: 0",
			wantErr: "positive number",
		},
		{
			name:    "hotfix_window_hours negative",
			yaml:    "quality:\n  hotfix_window_hours: -5",
			wantErr: "positive number",
		},
		{
			name:    "hotfix_window_hours too large",
			yaml:    "quality:\n  hotfix_window_hours: 9999",
			wantErr: "at most",
		},
		{
			name:    "hotfix_window_hours NaN",
			yaml:    "quality:\n  hotfix_window_hours: .nan",
			wantErr: "finite number",
		},
		{
			name:    "hotfix_window_hours +Inf",
			yaml:    "quality:\n  hotfix_window_hours: .inf",
			wantErr: "finite number",
		},
		{
			name:    "hotfix_window_hours valid boundary",
			yaml:    "quality:\n  hotfix_window_hours: 8760",
			wantErr: "",
		},
		{
			name:    "hotfix_window_hours valid small",
			yaml:    "quality:\n  hotfix_window_hours: 1",
			wantErr: "",
		},
		{
			name:    "project.id valid",
			yaml:    "project:\n  id: PVT_abc123XYZ",
			wantErr: "",
		},
		{
			name:    "project.id invalid prefix",
			yaml:    "project:\n  id: INVALID_abc123",
			wantErr: "project.id must match",
		},
		{
			name:    "project.id with special chars",
			yaml:    "project:\n  id: PVT_abc-123",
			wantErr: "project.id must match",
		},
		{
			name:    "project.id empty is OK",
			yaml:    "project:\n  id: \"\"",
			wantErr: "",
		},
		{
			name:    "status_field_id valid",
			yaml:    "project:\n  status_field_id: PVTSSF_abc123",
			wantErr: "",
		},
		{
			name:    "status_field_id invalid",
			yaml:    "project:\n  status_field_id: BAD_abc123",
			wantErr: "project.status_field_id must match",
		},
		{
			name:    "category_id valid",
			yaml:    "discussions:\n  category_id: DIC_abc123",
			wantErr: "",
		},
		{
			name:    "category_id invalid",
			yaml:    "discussions:\n  category_id: NOT_valid",
			wantErr: "discussions.category_id must match",
		},
		{
			name:    "category_id empty is OK",
			yaml:    "discussions:\n  category_id: \"\"",
			wantErr: "",
		},
		{
			name:    "workflow invalid",
			yaml:    "workflow: deploy",
			wantErr: "workflow must be",
		},
		{
			name: "full valid config",
			yaml: `workflow: local
project:
  id: PVT_kwDOTest
  status_field_id: PVTSSF_lADOTest
quality:
  hotfix_window_hours: 48
discussions:
  category_id: DIC_kwDOTest`,
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".gh-velocity.yml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0644); err != nil {
				t.Fatal(err)
			}

			// Suppress warnings during validation tests.
			origWarn := WarnFunc
			WarnFunc = func(string, ...any) {}
			defer func() { WarnFunc = origWarn }()

			_, err := Load(path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
