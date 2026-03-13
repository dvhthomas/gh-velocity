package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bitsbyme/gh-velocity/internal/model"
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
	if cfg.CycleTime.Strategy != model.StrategyIssue {
		t.Errorf("expected default cycle_time.strategy %q, got %q", model.StrategyIssue, cfg.CycleTime.Strategy)
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gh-velocity.yml")

	content := `
workflow: local
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
        - "label:defect"
    - name: feature
      match:
        - "label:feature"
  hotfix_window_hours: 48
project:
  url: "https://github.com/users/testuser/projects/1"
  status_field: "Status"
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
	if len(cfg.Quality.Categories) != 2 {
		t.Errorf("expected 2 categories, got %d", len(cfg.Quality.Categories))
	}
	if cfg.Quality.Categories[0].Name != "bug" || len(cfg.Quality.Categories[0].Matchers) != 2 {
		t.Errorf("expected bug category with 2 matchers, got %+v", cfg.Quality.Categories[0])
	}
	if cfg.Quality.HotfixWindowHours != 48 {
		t.Errorf("expected hotfix_window_hours 48, got %v", cfg.Quality.HotfixWindowHours)
	}
	if cfg.Project.URL != "https://github.com/users/testuser/projects/1" {
		t.Errorf("expected project.url, got %q", cfg.Project.URL)
	}
	if cfg.Project.StatusField != "Status" {
		t.Errorf("expected project.status_field 'Status', got %q", cfg.Project.StatusField)
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
  categories:
    - name: bug
      match:
        - "label:bug"
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

func TestLoad_DefaultCategories(t *testing.T) {
	cfg, err := Load("/nonexistent/.gh-velocity.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Quality.Categories) != 2 {
		t.Fatalf("expected 2 default categories, got %d", len(cfg.Quality.Categories))
	}
	bug := cfg.Quality.Categories[0]
	if bug.Name != "bug" || len(bug.Matchers) != 1 || bug.Matchers[0] != "label:bug" {
		t.Errorf("expected bug category with [label:bug], got %+v", bug)
	}
	feat := cfg.Quality.Categories[1]
	if feat.Name != "feature" || len(feat.Matchers) != 1 || feat.Matchers[0] != "label:enhancement" {
		t.Errorf("expected feature category with [label:enhancement], got %+v", feat)
	}
}

func TestLoad_ExplicitCategoriesPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gh-velocity.yml")
	content := `quality:
  categories:
    - name: regression
      match:
        - label:regression
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Explicit categories should NOT be overwritten by auto-generation.
	if len(cfg.Quality.Categories) != 1 {
		t.Fatalf("expected 1 explicit category, got %d", len(cfg.Quality.Categories))
	}
	reg := cfg.Quality.Categories[0]
	if reg.Name != "regression" || len(reg.Matchers) != 1 || reg.Matchers[0] != "label:regression" {
		t.Errorf("expected regression category preserved, got %+v", reg)
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

func TestParse_ValidConfig(t *testing.T) {
	data := []byte(`
workflow: local
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
        - "label:defect"
  hotfix_window_hours: 48
project:
  url: "https://github.com/users/testuser/projects/1"
  status_field: "Status"
`)
	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workflow != "local" {
		t.Errorf("expected workflow 'local', got %q", cfg.Workflow)
	}
	if len(cfg.Quality.Categories) != 1 || cfg.Quality.Categories[0].Name != "bug" {
		t.Errorf("expected 1 bug category, got %+v", cfg.Quality.Categories)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := Parse([]byte(`workflow: [invalid yaml`))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParse_UnknownKeysWarning(t *testing.T) {
	data := []byte(`
workflow: pr
posting:
  discussions: enabled
`)
	var warnings []string
	origWarn := WarnFunc
	WarnFunc = func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}
	defer func() { WarnFunc = origWarn }()

	_, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "posting") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about unknown key 'posting'")
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
			name:    "project.url valid user",
			yaml:    "project:\n  url: https://github.com/users/testuser/projects/1",
			wantErr: "",
		},
		{
			name:    "project.url valid org",
			yaml:    "project:\n  url: https://github.com/orgs/myorg/projects/42",
			wantErr: "",
		},
		{
			name:    "project.url invalid host",
			yaml:    "project:\n  url: https://gitlab.com/users/test/projects/1",
			wantErr: "project.url must be a github.com URL",
		},
		{
			name:    "project.url invalid path",
			yaml:    "project:\n  url: https://github.com/owner/repo",
			wantErr: "project.url must be",
		},
		{
			name:    "project.url non-numeric number",
			yaml:    "project:\n  url: https://github.com/users/test/projects/abc",
			wantErr: "project number",
		},
		{
			name:    "project.url empty is OK",
			yaml:    "project:\n  url: \"\"",
			wantErr: "",
		},
		{
			name:    "category valid",
			yaml:    "discussions:\n  category: General",
			wantErr: "",
		},
		{
			name:    "category empty is OK",
			yaml:    "discussions:\n  category: \"\"",
			wantErr: "",
		},
		{
			name:    "cycle_time.strategy issue",
			yaml:    "cycle_time:\n  strategy: issue",
			wantErr: "",
		},
		{
			name:    "cycle_time.strategy pr",
			yaml:    "cycle_time:\n  strategy: pr",
			wantErr: "",
		},
		{
			name:    "cycle_time.strategy project-board deprecated to issue",
			yaml:    "cycle_time:\n  strategy: project-board",
			wantErr: "",
		},
		{
			name:    "cycle_time.strategy invalid",
			yaml:    "cycle_time:\n  strategy: blended",
			wantErr: "cycle_time.strategy must be",
		},
		{
			name:    "categories valid",
			yaml:    "quality:\n  categories:\n    - name: bug\n      match:\n        - label:bug\n        - label:defect\n    - name: feature\n      match:\n        - label:enhancement\n    - name: regression\n      match:\n        - \"title:/^regression:/i\"",
			wantErr: "",
		},
		{
			name:    "categories invalid matcher",
			yaml:    "quality:\n  categories:\n    - name: bad\n      match:\n        - \"title:/[invalid\"",
			wantErr: "quality.categories.bad",
		},
		{
			name:    "categories invalid prefix",
			yaml:    "quality:\n  categories:\n    - name: x\n      match:\n        - unknown:foo",
			wantErr: "quality.categories.x",
		},
		{
			name:    "workflow invalid",
			yaml:    "workflow: deploy",
			wantErr: "workflow must be",
		},
		{
			name:    "lifecycle with project_status requires status_field",
			yaml:    "project:\n  url: https://github.com/users/test/projects/1\nlifecycle:\n  done:\n    project_status: [\"Done\"]",
			wantErr: "project.status_field is required",
		},
		{
			name:    "lifecycle with project_status requires project url",
			yaml:    "project:\n  status_field: Status\nlifecycle:\n  done:\n    project_status: [\"Done\"]",
			wantErr: "project.url is required",
		},
		{
			name: "lifecycle with project_status valid",
			yaml: `project:
  url: https://github.com/users/test/projects/1
  status_field: Status
lifecycle:
  done:
    project_status: ["Done", "Shipped"]
  backlog:
    project_status: ["Backlog"]`,
			wantErr: "",
		},
		{
			name:    "scope valid",
			yaml:    "scope:\n  query: 'repo:myorg/myrepo label:bug'",
			wantErr: "",
		},
		{
			name: "full valid config",
			yaml: `workflow: local
project:
  url: https://github.com/users/testuser/projects/1
  status_field: Status
quality:
  hotfix_window_hours: 48
discussions:
  category: General`,
			wantErr: "",
		},
		// --- Velocity config validation ---
		{
			name:    "velocity defaults valid",
			yaml:    "",
			wantErr: "",
		},
		{
			name:    "velocity unit invalid",
			yaml:    "velocity:\n  unit: stories",
			wantErr: "velocity.unit must be",
		},
		{
			name:    "velocity unit prs valid",
			yaml:    "velocity:\n  unit: prs",
			wantErr: "",
		},
		{
			name:    "velocity effort strategy invalid",
			yaml:    "velocity:\n  effort:\n    strategy: fibonacci",
			wantErr: "velocity.effort.strategy must be",
		},
		{
			name:    "velocity attribute empty matchers",
			yaml:    "velocity:\n  effort:\n    strategy: attribute",
			wantErr: "requires at least one matcher",
		},
		{
			name: "velocity attribute negative value",
			yaml: `velocity:
  effort:
    strategy: attribute
    attribute:
      - query: "label:bug"
        value: -1`,
			wantErr: "must be non-negative",
		},
		{
			name: "velocity attribute invalid query",
			yaml: `velocity:
  effort:
    strategy: attribute
    attribute:
      - query: "unknown:foo"
        value: 3`,
			wantErr: "velocity.effort.attribute[0].query",
		},
		{
			name: "velocity attribute valid",
			yaml: `velocity:
  effort:
    strategy: attribute
    attribute:
      - query: "label:size/S"
        value: 2
      - query: "label:size/M"
        value: 3`,
			wantErr: "",
		},
		{
			name: "velocity attribute value zero valid",
			yaml: `velocity:
  effort:
    strategy: attribute
    attribute:
      - query: "label:chore"
        value: 0`,
			wantErr: "",
		},
		{
			name:    "velocity numeric missing project_field",
			yaml:    "velocity:\n  effort:\n    strategy: numeric",
			wantErr: "numeric.project_field is required",
		},
		{
			name: "velocity numeric missing project url",
			yaml: `velocity:
  effort:
    strategy: numeric
    numeric:
      project_field: "Story Points"`,
			wantErr: "project.url is required",
		},
		{
			name: "velocity numeric valid",
			yaml: `project:
  url: https://github.com/users/test/projects/1
velocity:
  effort:
    strategy: numeric
    numeric:
      project_field: "Story Points"`,
			wantErr: "",
		},
		{
			name:    "velocity iteration strategy invalid",
			yaml:    "velocity:\n  iteration:\n    strategy: calendar",
			wantErr: "velocity.iteration.strategy must be",
		},
		{
			name: "velocity project-field missing field name",
			yaml: `project:
  url: https://github.com/users/test/projects/1
velocity:
  iteration:
    strategy: project-field`,
			wantErr: "velocity.iteration.project_field is required",
		},
		{
			name:    "velocity project-field missing project url",
			yaml:    "velocity:\n  iteration:\n    strategy: project-field\n    project_field: Sprint",
			wantErr: "project.url is required",
		},
		{
			name: "velocity project-field valid",
			yaml: `project:
  url: https://github.com/users/test/projects/1
velocity:
  iteration:
    strategy: project-field
    project_field: Sprint`,
			wantErr: "",
		},
		{
			name:    "velocity fixed missing length",
			yaml:    "velocity:\n  iteration:\n    strategy: fixed\n    fixed:\n      anchor: '2026-01-06'",
			wantErr: "fixed.length is required",
		},
		{
			name:    "velocity fixed invalid length",
			yaml:    "velocity:\n  iteration:\n    strategy: fixed\n    fixed:\n      length: '14x'\n      anchor: '2026-01-06'",
			wantErr: "invalid duration",
		},
		{
			name:    "velocity fixed missing anchor",
			yaml:    "velocity:\n  iteration:\n    strategy: fixed\n    fixed:\n      length: '14d'",
			wantErr: "fixed.anchor is required",
		},
		{
			name:    "velocity fixed invalid anchor",
			yaml:    "velocity:\n  iteration:\n    strategy: fixed\n    fixed:\n      length: '14d'\n      anchor: 'not-a-date'",
			wantErr: "must be a date",
		},
		{
			name: "velocity fixed valid",
			yaml: `velocity:
  iteration:
    strategy: fixed
    fixed:
      length: "14d"
      anchor: "2026-01-06"`,
			wantErr: "",
		},
		{
			name: "velocity fixed length weeks valid",
			yaml: `velocity:
  iteration:
    strategy: fixed
    fixed:
      length: "2w"
      anchor: "2026-01-06"`,
			wantErr: "",
		},
		{
			name:    "velocity iteration count zero",
			yaml:    "velocity:\n  iteration:\n    count: 0",
			wantErr: "count must be > 0",
		},
		{
			name:    "velocity iteration count negative",
			yaml:    "velocity:\n  iteration:\n    count: -1",
			wantErr: "count must be > 0",
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
