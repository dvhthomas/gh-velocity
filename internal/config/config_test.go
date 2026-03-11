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
	if cfg.CycleTime.Strategy != "issue" {
		t.Errorf("expected default cycle_time.strategy %q, got %q", "issue", cfg.CycleTime.Strategy)
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
	if len(cfg.Quality.BugLabels) != 2 {
		t.Errorf("expected 2 bug labels, got %d", len(cfg.Quality.BugLabels))
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

func TestLoad_DefaultCategoriesFromLabels(t *testing.T) {
	cfg, err := Load("/nonexistent/.gh-velocity.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Default config has bug_labels: ["bug"], feature_labels: ["enhancement"]
	// These should be auto-generated as categories.
	if len(cfg.Quality.Categories) == 0 {
		t.Fatal("expected auto-generated categories")
	}
	if len(cfg.Quality.Categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cfg.Quality.Categories))
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

func TestParse_CategoriesWithDefaultLabelsNoWarning(t *testing.T) {
	// Categories + default bug_labels/feature_labels should NOT warn.
	data := []byte(`quality:
  categories:
    - name: bug
      match:
        - label:bug
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
	for _, w := range warnings {
		if strings.Contains(w, "takes precedence") {
			t.Error("should not warn about precedence when legacy labels are only defaults")
		}
	}
}

func TestParse_CategoriesWithExplicitLegacyLabelsWarns(t *testing.T) {
	// Categories + explicit non-default bug_labels SHOULD warn.
	data := []byte(`quality:
  bug_labels: ["crash", "regression"]
  categories:
    - name: bug
      match:
        - label:bug
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
		if strings.Contains(w, "takes precedence") {
			found = true
		}
	}
	if !found {
		t.Error("expected precedence warning when explicit legacy labels and categories coexist")
	}
}

func TestParse_ValidConfig(t *testing.T) {
	data := []byte(`
workflow: local
quality:
  bug_labels: ["bug", "defect"]
  feature_labels: ["feature"]
  hotfix_window_hours: 48
project:
  url: "https://github.com/users/testuser/projects/1"
  status_field: "Status"
`)
	// Suppress warnings.
	origWarn := WarnFunc
	WarnFunc = func(string, ...any) {}
	defer func() { WarnFunc = origWarn }()

	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workflow != "local" {
		t.Errorf("expected workflow 'local', got %q", cfg.Workflow)
	}
	if len(cfg.Quality.BugLabels) != 2 {
		t.Errorf("expected 2 bug labels, got %d", len(cfg.Quality.BugLabels))
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
			name:    "cycle_time.strategy project-board with project",
			yaml:    "cycle_time:\n  strategy: project-board\nproject:\n  url: https://github.com/users/test/projects/1",
			wantErr: "",
		},
		{
			name:    "cycle_time.strategy project-board without project",
			yaml:    "cycle_time:\n  strategy: project-board",
			wantErr: "requires project.url",
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
