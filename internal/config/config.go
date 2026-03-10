// Package config handles parsing and validation of .gh-velocity.yml.
package config

import (
	"fmt"
	"math"
	"os"
	"regexp"

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

var (
	projectIDPattern     = regexp.MustCompile(`^PVT_[a-zA-Z0-9]+$`)
	statusFieldIDPattern = regexp.MustCompile(`^PVTSSF_[a-zA-Z0-9]+$`)
	categoryIDPattern    = regexp.MustCompile(`^DIC_[a-zA-Z0-9]+$`)
)

// WarnFunc is called for non-fatal warnings (e.g., unknown config keys).
// Defaults to log.Warn.
var WarnFunc = func(format string, args ...any) {
	log.Warn(format, args...)
}

// Config represents the .gh-velocity.yml configuration.
type Config struct {
	Workflow    string            `yaml:"workflow" json:"workflow"`
	Project     ProjectConfig     `yaml:"project" json:"project"`
	Statuses    StatusConfig      `yaml:"statuses" json:"statuses"`
	Fields      FieldsConfig      `yaml:"fields" json:"fields"`
	Quality     QualityConfig     `yaml:"quality" json:"quality"`
	Discussions DiscussionsConfig `yaml:"discussions" json:"discussions"`
	CommitRef   CommitRefConfig   `yaml:"commit_ref" json:"commit_ref"`
	CycleTime   CycleTimeConfig   `yaml:"cycle_time" json:"cycle_time"`
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

type ProjectConfig struct {
	ID            string `yaml:"id" json:"id"`
	StatusFieldID string `yaml:"status_field_id" json:"status_field_id"`
}

type StatusConfig struct {
	Backlog    string `yaml:"backlog" json:"backlog"`
	Ready      string `yaml:"ready" json:"ready"`
	InProgress string `yaml:"in_progress" json:"in_progress"`
	InReview   string `yaml:"in_review" json:"in_review"`
	Done       string `yaml:"done" json:"done"`

	// BacklogLabels are issue labels that indicate work has NOT started.
	// When an issue has any of these labels, cycle time is suppressed.
	// Example: ["backlog", "icebox", "deferred"]
	BacklogLabels []string `yaml:"backlog_labels" json:"backlog_labels"`

	// ActiveLabels are issue labels that indicate work HAS started.
	// When one of these labels is added to an issue, that becomes a
	// cycle time signal. Example: ["in-progress", "in progress", "wip"]
	// This is an alternative to Projects v2 for repos that use labels
	// to track status (common in OSS).
	ActiveLabels []string `yaml:"active_labels" json:"active_labels"`
}

type FieldsConfig struct {
	StartDate  string `yaml:"start_date" json:"start_date"`
	TargetDate string `yaml:"target_date" json:"target_date"`
}

type QualityConfig struct {
	BugLabels         []string               `yaml:"bug_labels" json:"bug_labels"`
	FeatureLabels     []string               `yaml:"feature_labels" json:"feature_labels"`
	Categories        []model.CategoryConfig `yaml:"categories" json:"categories"`
	HotfixWindowHours float64                `yaml:"hotfix_window_hours" json:"hotfix_window_hours"`
}

type DiscussionsConfig struct {
	CategoryID string `yaml:"category_id" json:"category_id"`
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

	// Parse into raw map first for unknown key detection.
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	warnUnknownKeysFromMap(raw)

	// Parse into typed struct.
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
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
		Quality: QualityConfig{
			BugLabels:         []string{"bug"},
			FeatureLabels:     []string{"enhancement"},
			HotfixWindowHours: DefaultHotfixWindowHours,
		},
		Statuses: StatusConfig{
			Backlog:    "Backlog",
			Ready:      "Ready",
			InProgress: "In progress",
			InReview:   "In review",
			Done:       "Done",
		},
	}
}

// knownTopLevelKeys lists the YAML keys that map to Config struct fields.
var knownTopLevelKeys = map[string]bool{
	"workflow":    true,
	"project":     true,
	"statuses":    true,
	"fields":      true,
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
	if cfg.CycleTime.Strategy == "project-board" && cfg.Project.ID == "" {
		return fmt.Errorf("config: cycle_time.strategy \"project-board\" requires project.id to be set")
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

	// GraphQL node ID validation (only when set).
	if id := cfg.Project.ID; id != "" {
		if !projectIDPattern.MatchString(id) {
			return fmt.Errorf("config: project.id must match %s, got %q", projectIDPattern.String(), id)
		}
	}
	if id := cfg.Project.StatusFieldID; id != "" {
		if !statusFieldIDPattern.MatchString(id) {
			return fmt.Errorf("config: project.status_field_id must match %s, got %q", statusFieldIDPattern.String(), id)
		}
	}
	if id := cfg.Discussions.CategoryID; id != "" {
		if !categoryIDPattern.MatchString(id) {
			return fmt.Errorf("config: discussions.category_id must match %s, got %q", categoryIDPattern.String(), id)
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
// is emitted if legacy labels are also present). Otherwise, categories are
// auto-generated from bug_labels/feature_labels for backward compatibility.
func resolveCategories(cfg *Config) {
	hasLegacy := len(cfg.Quality.BugLabels) > 0 || len(cfg.Quality.FeatureLabels) > 0

	if len(cfg.Quality.Categories) > 0 {
		if hasLegacy {
			WarnFunc("config: both 'categories' and 'bug_labels'/'feature_labels' are set; 'categories' takes precedence")
		}
		return
	}

	// Auto-generate from legacy labels (including defaults).
	cfg.Quality.Categories = classify.FromLegacyLabels(cfg.Quality.BugLabels, cfg.Quality.FeatureLabels)
}
