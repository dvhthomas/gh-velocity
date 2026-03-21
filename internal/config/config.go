// Package config handles parsing and validation of .gh-velocity.yml.
package config

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/classify"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigFile         = ".gh-velocity.yml"
	DefaultWorkflow           = "pr"
	DefaultHotfixWindowHours  = 72
	DefaultBugRatioThreshold  = 0.20
	MaxBugRatioThreshold      = 0.60      // above this is suspicious (data quality issue, not real bug ratio)
	MaxConfigFileSize         = 64 * 1024 // 64 KB
	MaxHotfixWindowHours      = 8760      // 1 year in hours
	DefaultAPIThrottleSeconds = 2
)

// WarnFunc is called for non-fatal warnings (e.g., unknown config keys).
// Defaults to log.Warn.
var WarnFunc = func(format string, args ...any) {
	log.Warn(format, args...)
}

// Config represents the .gh-velocity.yml configuration.
type Config struct {
	Workflow     string            `yaml:"workflow" json:"workflow"`
	Scope        ScopeConfig       `yaml:"scope" json:"scope"`
	Project      ProjectConfig     `yaml:"project" json:"project"`
	Lifecycle    LifecycleConfig   `yaml:"lifecycle" json:"lifecycle"`
	Quality      QualityConfig     `yaml:"quality" json:"quality"`
	Discussions  DiscussionsConfig `yaml:"discussions" json:"discussions"`
	CommitRef    CommitRefConfig   `yaml:"commit_ref" json:"commit_ref"`
	CycleTime    CycleTimeConfig   `yaml:"cycle_time" json:"cycle_time"`
	Effort       EffortConfig      `yaml:"effort" json:"effort"`
	WIP          WIPConfig         `yaml:"wip" json:"wip"`
	Velocity     VelocityConfig    `yaml:"velocity" json:"velocity"`
	ExcludeUsers []string          `yaml:"exclude_users" json:"exclude_users"`
	// APIThrottleSeconds is the minimum delay in seconds between GitHub search
	// API calls. Prevents triggering GitHub's secondary (abuse) rate limits
	// which have undocumented thresholds and multi-minute lockouts. Default: 2.
	// Set to 0 to disable throttling (not recommended).
	APIThrottleSeconds *int `yaml:"api_throttle_seconds" json:"api_throttle_seconds"`
}

// WIPConfig controls work-in-progress limit thresholds.
type WIPConfig struct {
	TeamLimit   *float64 `yaml:"team_limit" json:"team_limit"`
	PersonLimit *float64 `yaml:"person_limit" json:"person_limit"`
	Bots        []string `yaml:"bots" json:"bots"` // additional bot logins (case-insensitive exact match)
}

// VelocityConfig controls how velocity (effort per iteration) is measured.
type VelocityConfig struct {
	Unit      string          `yaml:"unit" json:"unit"` // "issues" or "prs"
	Effort    EffortConfig    `yaml:"effort" json:"effort"`
	Iteration IterationConfig `yaml:"iteration" json:"iteration"`
}

// EffortConfig controls how effort is assigned to work items.
type EffortConfig struct {
	Strategy  string              `yaml:"strategy" json:"strategy"` // "count", "attribute", "numeric"
	Attribute []EffortMatcher     `yaml:"attribute" json:"attribute"`
	Numeric   NumericEffortConfig `yaml:"numeric" json:"numeric"`
}

// EffortMatcher maps a classify.Matcher query to an effort value.
type EffortMatcher struct {
	Query string  `yaml:"query" json:"query"` // classify.Matcher syntax
	Value float64 `yaml:"value" json:"value"`
}

// NumericEffortConfig identifies a Number field on a project board.
type NumericEffortConfig struct {
	ProjectField string `yaml:"project_field" json:"project_field"`
}

// IterationConfig controls how iteration boundaries are determined.
type IterationConfig struct {
	Strategy     string               `yaml:"strategy" json:"strategy"` // "project-field" or "fixed"
	ProjectField string               `yaml:"project_field" json:"project_field"`
	Fixed        FixedIterationConfig `yaml:"fixed" json:"fixed"`
	Count        int                  `yaml:"count" json:"count"` // default 3; higher values increase API consumption
}

// FixedIterationConfig defines calendar-based iteration boundaries.
type FixedIterationConfig struct {
	Length string `yaml:"length" json:"length"` // e.g., "14d"
	Anchor string `yaml:"anchor" json:"anchor"` // e.g., "2026-01-06"
}

// ScopeConfig holds the user's scope query — a GitHub search query fragment
// that determines which items (issues or PRs) all metrics operate on.
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

// LifecycleStage defines the query qualifiers and label matchers for a workflow stage.
type LifecycleStage struct {
	// Query is appended to scope for REST search API calls (e.g., "is:closed").
	Query string `yaml:"query" json:"query"`
	// Match lists classify.Matcher patterns (e.g., "label:in-progress") for
	// client-side lifecycle grouping. Used by the issue cycle time strategy
	// and the WIP command.
	Match []string `yaml:"match" json:"match,omitempty"`
}

// LifecycleConfig defines lifecycle stages — where an item is in its workflow
// journey. Labels are the sole lifecycle signal; their timestamps are immutable.
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
	// Values: model.StrategyIssue (default), model.StrategyPR.
	Strategy string `yaml:"strategy" json:"strategy"`
}

// CommitRefConfig controls the commit-ref strategy behavior.
type CommitRefConfig struct {
	Patterns []string `yaml:"patterns" json:"patterns"` // ["closes"] or ["closes", "refs"]
}

type QualityConfig struct {
	Categories        []model.CategoryConfig `yaml:"categories" json:"categories"`
	HotfixWindowHours float64                `yaml:"hotfix_window_hours" json:"hotfix_window_hours"`
	BugRatioThreshold float64                `yaml:"bug_ratio_threshold" json:"bug_ratio_threshold"`
}

type DiscussionsConfig struct {
	Category string `yaml:"category" json:"category"`
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

	// Apply defaults for fields that YAML zeros when parent key is present.
	if cfg.Quality.BugRatioThreshold == 0 {
		cfg.Quality.BugRatioThreshold = DefaultBugRatioThreshold
	}

	// Migrate effort from velocity.effort to top-level effort.
	migrateEffort(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// migrateEffort promotes velocity.effort to the top-level effort key.
// It handles three cases:
//  1. Only velocity.effort set → copy to top-level, emit deprecation warning
//  2. Both set → top-level wins, emit "ignoring velocity.effort" warning
//  3. Only top-level set → no migration needed
//
// After migration, velocity.effort is always synced from the top-level effort
// so the velocity pipeline reads from the canonical location.
func migrateEffort(cfg *Config) {
	topLevelSet := cfg.Effort.Strategy != "" && cfg.Effort.Strategy != "count"
	nestedSet := cfg.Velocity.Effort.Strategy != "" && cfg.Velocity.Effort.Strategy != "count"

	// More precise detection: if the top-level effort is still just the default
	// and the nested one was explicitly configured with non-default values.
	topLevelHasAttribute := len(cfg.Effort.Attribute) > 0
	topLevelHasNumeric := cfg.Effort.Numeric.ProjectField != ""
	nestedHasAttribute := len(cfg.Velocity.Effort.Attribute) > 0
	nestedHasNumeric := cfg.Velocity.Effort.Numeric.ProjectField != ""

	topLevelExplicit := topLevelSet || topLevelHasAttribute || topLevelHasNumeric
	nestedExplicit := nestedSet || nestedHasAttribute || nestedHasNumeric

	switch {
	case !topLevelExplicit && nestedExplicit:
		// Case 1: Only velocity.effort is set — migrate up.
		cfg.Effort = cfg.Velocity.Effort
		WarnFunc("config: velocity.effort is deprecated; move effort config to the top-level \"effort\" key")
	case topLevelExplicit && nestedExplicit:
		// Case 2: Both set — top-level wins.
		WarnFunc("config: ignoring velocity.effort because top-level effort is configured; remove velocity.effort to silence this warning")
	}

	// Always sync: velocity pipeline reads from cfg.Velocity.Effort.
	cfg.Velocity.Effort = cfg.Effort
}

// Defaults returns a Config with default values. Used by config subcommands
// and test fixtures. Non-config commands require a config file.
func Defaults() *Config {
	return defaults()
}

func defaults() *Config {
	return &Config{
		Workflow: DefaultWorkflow,
		Effort: EffortConfig{
			Strategy: "count",
		},
		CycleTime: CycleTimeConfig{
			Strategy: model.StrategyIssue,
		},
		Lifecycle: LifecycleConfig{
			Backlog:    LifecycleStage{Query: "is:open"},
			InProgress: LifecycleStage{Query: "is:open"},
			InReview:   LifecycleStage{Query: "is:open"},
			Done:       LifecycleStage{Query: "is:closed"},
			// Released: no default — tag-based discovery, no query needed.
		},
		Quality: QualityConfig{
			Categories: []model.CategoryConfig{
				{Name: "bug", Matchers: []string{"label:bug"}},
				{Name: "feature", Matchers: []string{"label:enhancement"}},
			},
			HotfixWindowHours: DefaultHotfixWindowHours,
			BugRatioThreshold: DefaultBugRatioThreshold,
		},
		Velocity: VelocityConfig{
			Unit: "issues",
			Effort: EffortConfig{
				Strategy: "count",
			},
			Iteration: IterationConfig{
				Count: 6,
			},
		},
	}
}

// knownTopLevelKeys lists the YAML keys that map to Config struct fields.
var knownTopLevelKeys = map[string]bool{
	"workflow":             true,
	"scope":                true,
	"project":              true,
	"lifecycle":            true,
	"quality":              true,
	"discussions":          true,
	"commit_ref":           true,
	"cycle_time":           true,
	"effort":              true,
	"wip":                 true,
	"exclude_users":        true,
	"velocity":             true,
	"api_throttle_seconds": true,
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

	// bug_ratio_threshold: must be in (0, MaxBugRatioThreshold).
	if cfg.Quality.BugRatioThreshold <= 0 || cfg.Quality.BugRatioThreshold >= 1.0 {
		return fmt.Errorf("config: quality.bug_ratio_threshold must be between 0 and 1 exclusive, got %v", cfg.Quality.BugRatioThreshold)
	}
	if cfg.Quality.BugRatioThreshold >= MaxBugRatioThreshold {
		return fmt.Errorf("config: quality.bug_ratio_threshold must be less than %.0f%% (the suspicious threshold that indicates a data quality issue), got %.0f%%",
			MaxBugRatioThreshold*100, cfg.Quality.BugRatioThreshold*100)
	}

	// cycle_time.strategy: must be a known value.
	switch cfg.CycleTime.Strategy {
	case model.StrategyIssue, model.StrategyPR:
		// valid
	default:
		return fmt.Errorf("config: cycle_time.strategy must be %q or %q, got %q", model.StrategyIssue, model.StrategyPR, cfg.CycleTime.Strategy)
	}

	// Issue strategy requires lifecycle.in-progress.match for cycle time signals.
	if cfg.CycleTime.Strategy == model.StrategyIssue && len(cfg.Lifecycle.InProgress.Match) == 0 {
		WarnFunc("config: issue strategy has no lifecycle.in-progress.match — cycle time will be unavailable. Run: gh velocity config preflight --write")
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

	if err := validateEffort(&cfg.Effort, cfg.Project.URL); err != nil {
		return err
	}

	if err := validateVelocity(&cfg.Velocity, cfg.Project.URL); err != nil {
		return err
	}

	return nil
}

// APIThrottleDuration returns the configured delay between search API calls.
// Returns 0 (no throttle) if not set. The preflight command recommends
// api_throttle_seconds: 2 when generating a config.
func (c *Config) APIThrottleDuration() time.Duration {
	if c.APIThrottleSeconds != nil {
		return time.Duration(*c.APIThrottleSeconds) * time.Second
	}
	return 0
}

// durationPattern matches duration strings like "14d", "7d", "1w".
var durationPattern = regexp.MustCompile(`^(\d+)([dw])$`)

// ParseFixedLength parses a fixed iteration length string like "14d" or "2w"
// into a time.Duration. Exported for use by the period computation engine.
func ParseFixedLength(s string) (time.Duration, error) {
	m := durationPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid duration %q: expected format like \"14d\" or \"2w\"", s)
	}
	n, _ := strconv.Atoi(m[1])
	switch m[2] {
	case "d":
		return time.Duration(n) * 24 * time.Hour, nil
	case "w":
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("invalid duration unit in %q", s)
}

// validateEffort validates an EffortConfig. Called on the top-level effort
// (which is the canonical location after migration).
func validateEffort(e *EffortConfig, projectURL string) error {
	switch e.Strategy {
	case "count":
		// no additional validation
	case "attribute":
		if len(e.Attribute) == 0 {
			return fmt.Errorf("config: effort.attribute requires at least one matcher")
		}
		hasFieldMatcher := false
		for i, m := range e.Attribute {
			if m.Value < 0 {
				return fmt.Errorf("config: effort.attribute[%d].value must be non-negative, got %v", i, m.Value)
			}
			if _, err := classify.ParseMatcher(m.Query); err != nil {
				return fmt.Errorf("config: effort.attribute[%d].query: %w", i, err)
			}
			if strings.HasPrefix(m.Query, "field:") {
				hasFieldMatcher = true
			}
		}
		if hasFieldMatcher && projectURL == "" {
			return fmt.Errorf("config: project.url is required when effort.attribute uses \"field:\" matchers (requires project board access)")
		}
	case "numeric":
		if e.Numeric.ProjectField == "" {
			return fmt.Errorf("config: effort.numeric.project_field is required")
		}
		if projectURL == "" {
			return fmt.Errorf("config: project.url is required when effort.strategy is \"numeric\"")
		}
	default:
		return fmt.Errorf("config: effort.strategy must be \"count\", \"attribute\", or \"numeric\", got %q", e.Strategy)
	}
	return nil
}

func validateVelocity(v *VelocityConfig, projectURL string) error {
	// unit
	switch v.Unit {
	case "issues", "prs":
		// valid
	default:
		return fmt.Errorf("config: velocity.unit must be \"issues\" or \"prs\", got %q", v.Unit)
	}

	// Effort is validated at top level via validateEffort(); velocity.effort
	// is synced from the top-level effort in migrateEffort().

	// iteration.strategy
	switch v.Iteration.Strategy {
	case "project-field":
		if v.Iteration.ProjectField == "" {
			return fmt.Errorf("config: velocity.iteration.project_field is required when strategy is \"project-field\"")
		}
		if projectURL == "" {
			return fmt.Errorf("config: project.url is required when velocity.iteration.strategy is \"project-field\"")
		}
	case "fixed":
		if v.Iteration.Fixed.Length == "" {
			return fmt.Errorf("config: velocity.iteration.fixed.length is required when strategy is \"fixed\"")
		}
		if _, err := ParseFixedLength(v.Iteration.Fixed.Length); err != nil {
			return fmt.Errorf("config: velocity.iteration.fixed.length: %w", err)
		}
		if v.Iteration.Fixed.Anchor == "" {
			return fmt.Errorf("config: velocity.iteration.fixed.anchor is required when strategy is \"fixed\"")
		}
		if _, err := time.Parse(time.DateOnly, v.Iteration.Fixed.Anchor); err != nil {
			return fmt.Errorf("config: velocity.iteration.fixed.anchor must be a date (YYYY-MM-DD), got %q", v.Iteration.Fixed.Anchor)
		}
	case "":
		// No iteration strategy specified — OK (velocity section may be defaults-only)
	default:
		return fmt.Errorf("config: velocity.iteration.strategy must be \"project-field\" or \"fixed\", got %q", v.Iteration.Strategy)
	}

	// count
	if v.Iteration.Count <= 0 {
		return fmt.Errorf("config: velocity.iteration.count must be > 0, got %d", v.Iteration.Count)
	}

	return nil
}
