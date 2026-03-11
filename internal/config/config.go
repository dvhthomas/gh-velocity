// Package config handles parsing and validation of .gh-velocity.yml.
package config

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/classify"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigFile        = ".gh-velocity.yml"
	DefaultWorkflow          = "pr"
	DefaultHotfixWindowHours = 72
	MaxConfigFileSize        = 64 * 1024 // 64 KB
	MaxHotfixWindowHours     = 8760      // 1 year in hours
)

// WarnFunc is called for non-fatal warnings (e.g., unknown config keys).
// Defaults to log.Warn.
var WarnFunc = func(format string, args ...any) {
	log.Warn(format, args...)
}

// Config represents the .gh-velocity.yml configuration.
type Config struct {
	Workflow    string            `yaml:"workflow" json:"workflow"`
	Scope       ScopeConfig       `yaml:"scope" json:"scope"`
	Project     ProjectConfig     `yaml:"project" json:"project"`
	Lifecycle   LifecycleConfig   `yaml:"lifecycle" json:"lifecycle"`
	Quality     QualityConfig     `yaml:"quality" json:"quality"`
	Discussions DiscussionsConfig `yaml:"discussions" json:"discussions"`
	CommitRef   CommitRefConfig   `yaml:"commit_ref" json:"commit_ref"`
	CycleTime   CycleTimeConfig   `yaml:"cycle_time" json:"cycle_time"`
}

// ScopeConfig holds the user's scope query — a GitHub search query fragment.
type ScopeConfig struct {
	Query string `yaml:"query" json:"query"`
}

// ProjectConfig identifies a GitHub Projects v2 board.
type ProjectConfig struct {
	// URL is the GitHub project URL, e.g. "https://github.com/users/dvhthomas/projects/1"
	URL string `yaml:"url" json:"url"`
	// StatusField is the visible name of the status field, e.g. "Status"
	StatusField string `yaml:"status_field" json:"status_field"`
}

// LifecycleStage defines the query qualifiers and/or project statuses for a workflow stage.
type LifecycleStage struct {
	// Query is appended to scope for REST search API calls (e.g., "is:closed").
	Query string `yaml:"query" json:"query"`
	// ProjectStatus lists project board status values for GraphQL filtering.
	ProjectStatus []string `yaml:"project_status" json:"project_status"`
}

// LifecycleConfig defines the lifecycle stages for workflow-based filtering.
// Commands know which stage to use; users define what each stage means.
type LifecycleConfig struct {
	Backlog    LifecycleStage `yaml:"backlog" json:"backlog"`
	InProgress LifecycleStage `yaml:"in-progress" json:"in-progress"`
	InReview   LifecycleStage `yaml:"in-review" json:"in-review"`
	Done       LifecycleStage `yaml:"done" json:"done"`
	Released   LifecycleStage `yaml:"released" json:"released"`
}

// CycleTimeConfig controls how cycle time is measured.
type CycleTimeConfig struct {
	// Strategy selects the cycle-time measurement approach.
	// Values: "issue" (default), "pr", "project-board".
	Strategy string `yaml:"strategy" json:"strategy"`
}

// CommitRefConfig controls the commit-ref strategy behavior.
type CommitRefConfig struct {
	Patterns []string `yaml:"patterns" json:"patterns"` // ["closes"] or ["closes", "refs"]
}

type QualityConfig struct {
	BugLabels         []string               `yaml:"bug_labels" json:"bug_labels"`
	FeatureLabels     []string               `yaml:"feature_labels" json:"feature_labels"`
	Categories        []model.CategoryConfig `yaml:"categories" json:"categories"`
	HotfixWindowHours float64                `yaml:"hotfix_window_hours" json:"hotfix_window_hours"`
}

type DiscussionsConfig struct {
	Category string `yaml:"category" json:"category"`
}

// Load reads and parses the config file. Returns default config if file doesn't exist.
func Load(path string) (*Config, error) {
	cfg := defaults()

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		resolveCategories(cfg)
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: stat %s: %w", path, err)
	}
	if info.Size() > MaxConfigFileSize {
		return nil, fmt.Errorf("config: %s exceeds maximum size of %d bytes", path, MaxConfigFileSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	return Parse(data)
}

// Parse parses and validates a YAML config from raw bytes.
// This enables round-trip validation without temp files.
func Parse(data []byte) (*Config, error) {
	cfg := defaults()

	// Parse into raw map first for unknown key detection.
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("config: parse: %w", err)
	}
	warnUnknownKeysFromMap(raw)

	// Parse into typed struct.
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse: %w", err)
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	resolveCategories(cfg)

	return cfg, nil
}

// Defaults returns a Config with default values. Exported for use when
// no config file is available (e.g., running with --repo outside a git checkout).
func Defaults() *Config {
	cfg := defaults()
	resolveCategories(cfg)
	return cfg
}

func defaults() *Config {
	return &Config{
		Workflow: DefaultWorkflow,
		CycleTime: CycleTimeConfig{
			Strategy: "issue",
		},
		Lifecycle: LifecycleConfig{
			Backlog:    LifecycleStage{Query: "is:open"},
			InProgress: LifecycleStage{Query: "is:open"},
			InReview:   LifecycleStage{Query: "is:open"},
			Done:       LifecycleStage{Query: "is:closed"},
			// Released: no default — tag-based discovery, no query needed.
		},
		Quality: QualityConfig{
			BugLabels:         []string{"bug"},
			FeatureLabels:     []string{"enhancement"},
			HotfixWindowHours: DefaultHotfixWindowHours,
		},
	}
}

// knownTopLevelKeys lists the YAML keys that map to Config struct fields.
var knownTopLevelKeys = map[string]bool{
	"workflow":    true,
	"scope":       true,
	"project":     true,
	"lifecycle":   true,
	"quality":     true,
	"discussions": true,
	"commit_ref":  true,
	"cycle_time":  true,
}

// warnUnknownKeysFromMap warns about any top-level keys in the parsed map
// that don't correspond to known Config fields.
func warnUnknownKeysFromMap(raw map[string]any) {
	for key := range raw {
		if !knownTopLevelKeys[key] {
			WarnFunc("config: unknown key %q (ignored)", key)
		}
	}
}

// projectURLPattern matches GitHub project URLs:
//   - https://github.com/users/{user}/projects/{N}
//   - https://github.com/orgs/{org}/projects/{N}
func validateProjectURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("config: project.url is not a valid URL: %w", err)
	}
	if u.Host != "github.com" {
		return fmt.Errorf("config: project.url must be a github.com URL, got host %q", u.Host)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Expected: users/{user}/projects/{N} or orgs/{org}/projects/{N}
	if len(parts) != 4 || (parts[0] != "users" && parts[0] != "orgs") || parts[2] != "projects" {
		return fmt.Errorf("config: project.url must be https://github.com/users/{user}/projects/{N} or https://github.com/orgs/{org}/projects/{N}, got %q", rawURL)
	}
	if _, err := strconv.Atoi(parts[3]); err != nil {
		return fmt.Errorf("config: project.url must end with a project number, got %q", parts[3])
	}
	return nil
}

func validate(cfg *Config) error {
	switch cfg.Workflow {
	case "pr", "local":
		// valid
	default:
		return fmt.Errorf("config: workflow must be \"pr\" or \"local\", got %q", cfg.Workflow)
	}

	// hotfix_window_hours: must be finite, positive, and within range.
	if math.IsNaN(cfg.Quality.HotfixWindowHours) || math.IsInf(cfg.Quality.HotfixWindowHours, 0) {
		return fmt.Errorf("config: quality.hotfix_window_hours must be a finite number")
	}
	if cfg.Quality.HotfixWindowHours <= 0 {
		return fmt.Errorf("config: quality.hotfix_window_hours must be a positive number, got %v", cfg.Quality.HotfixWindowHours)
	}
	if cfg.Quality.HotfixWindowHours > MaxHotfixWindowHours {
		return fmt.Errorf("config: quality.hotfix_window_hours must be at most %d, got %v", MaxHotfixWindowHours, cfg.Quality.HotfixWindowHours)
	}

	// cycle_time.strategy: must be a known value.
	switch cfg.CycleTime.Strategy {
	case "issue", "pr", "project-board":
		// valid
	default:
		return fmt.Errorf("config: cycle_time.strategy must be \"issue\", \"pr\", or \"project-board\", got %q", cfg.CycleTime.Strategy)
	}
	if cfg.CycleTime.Strategy == "project-board" && cfg.Project.URL == "" {
		return fmt.Errorf("config: cycle_time.strategy \"project-board\" requires project.url to be set")
	}

	// commit_ref.patterns: validate values.
	for _, p := range cfg.CommitRef.Patterns {
		switch p {
		case "closes", "refs":
			// valid
		default:
			return fmt.Errorf("config: commit_ref.patterns must contain \"closes\" or \"refs\", got %q", p)
		}
	}

	// Project URL validation (only when set).
	if cfg.Project.URL != "" {
		if err := validateProjectURL(cfg.Project.URL); err != nil {
			return err
		}
	}

	// status_field is required when any lifecycle stage uses project_status.
	hasProjectStatus := len(cfg.Lifecycle.Backlog.ProjectStatus) > 0 ||
		len(cfg.Lifecycle.InProgress.ProjectStatus) > 0 ||
		len(cfg.Lifecycle.InReview.ProjectStatus) > 0 ||
		len(cfg.Lifecycle.Done.ProjectStatus) > 0 ||
		len(cfg.Lifecycle.Released.ProjectStatus) > 0
	if hasProjectStatus && cfg.Project.StatusField == "" {
		return fmt.Errorf("config: project.status_field is required when lifecycle stages use project_status")
	}
	if hasProjectStatus && cfg.Project.URL == "" {
		return fmt.Errorf("config: project.url is required when lifecycle stages use project_status")
	}

	// Discussions category name validation.
	if name := cfg.Discussions.Category; name != "" {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("config: discussions.category must be a non-empty name, got %q", name)
		}
	}

	// Validate category matchers if present.
	for _, cat := range cfg.Quality.Categories {
		for _, m := range cat.Matchers {
			if _, err := classify.ParseMatcher(m); err != nil {
				return fmt.Errorf("config: quality.categories.%s: %w", cat.Name, err)
			}
		}
	}

	return nil
}


// resolveCategories ensures cfg.Quality.Categories is populated.
// If the user specified explicit categories, those are used (and a warning
// is emitted if legacy labels are also present in the YAML). Otherwise,
// categories are auto-generated from bug_labels/feature_labels for backward compatibility.
func resolveCategories(cfg *Config) {
	if len(cfg.Quality.Categories) > 0 {
		// Legacy labels from defaults() are always present; only warn if they
		// differ from the defaults, indicating the user explicitly set them.
		explicitBug := !slicesEqual(cfg.Quality.BugLabels, []string{"bug"})
		explicitFeature := !slicesEqual(cfg.Quality.FeatureLabels, []string{"enhancement"})
		if explicitBug || explicitFeature {
			WarnFunc("config: both 'categories' and 'bug_labels'/'feature_labels' are set; 'categories' takes precedence")
		}
		return
	}

	// Auto-generate from legacy labels (including defaults).
	cfg.Quality.Categories = classify.FromLegacyLabels(cfg.Quality.BugLabels, cfg.Quality.FeatureLabels)
}

// slicesEqual reports whether two string slices are identical.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
