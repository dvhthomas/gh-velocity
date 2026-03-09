// Package config handles parsing and validation of .gh-velocity.yml.
package config

import (
	"fmt"
	"math"
	"os"
	"regexp"

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
// Defaults to fmt.Fprintf(os.Stderr, ...).
var WarnFunc = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}

// Config represents the .gh-velocity.yml configuration.
type Config struct {
	Workflow    string            `yaml:"workflow" json:"workflow"`
	Project     ProjectConfig     `yaml:"project" json:"project"`
	Statuses    StatusConfig      `yaml:"statuses" json:"statuses"`
	Fields      FieldsConfig      `yaml:"fields" json:"fields"`
	Quality     QualityConfig     `yaml:"quality" json:"quality"`
	Discussions DiscussionsConfig `yaml:"discussions" json:"discussions"`
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
}

type FieldsConfig struct {
	StartDate  string `yaml:"start_date" json:"start_date"`
	TargetDate string `yaml:"target_date" json:"target_date"`
}

type QualityConfig struct {
	BugLabels         []string `yaml:"bug_labels" json:"bug_labels"`
	FeatureLabels     []string `yaml:"feature_labels" json:"feature_labels"`
	HotfixWindowHours float64  `yaml:"hotfix_window_hours" json:"hotfix_window_hours"`
}

type DiscussionsConfig struct {
	CategoryID string `yaml:"category_id" json:"category_id"`
}

// Load reads and parses the config file. Returns default config if file doesn't exist.
func Load(path string) (*Config, error) {
	cfg := defaults()

	info, err := os.Stat(path)
	if os.IsNotExist(err) {
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

	return cfg, nil
}

// Defaults returns a Config with default values. Exported for use when
// no config file is available (e.g., running with --repo outside a git checkout).
func Defaults() *Config {
	return defaults()
}

func defaults() *Config {
	return &Config{
		Workflow: DefaultWorkflow,
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
}

// warnUnknownKeysFromMap warns about any top-level keys in the parsed map
// that don't correspond to known Config fields.
func warnUnknownKeysFromMap(raw map[string]any) {
	for key := range raw {
		if !knownTopLevelKeys[key] {
			WarnFunc("config: warning: unknown key %q (ignored)\n", key)
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

	return nil
}
