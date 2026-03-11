package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/classify"
	"github.com/bitsbyme/gh-velocity/internal/config"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

func newConfigPreflightCmd() *cobra.Command {
	var (
		writeFlag   bool
		projectFlag int
	)

	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Analyze a repo and recommend a .gh-velocity.yml configuration",
		Long: `Preflight queries your repository to build a smart starting configuration.

It inspects:
  - Labels (to detect bug, feature, and status labels)
  - A specific Projects v2 board (to find board IDs and status fields)
  - Recent merged PRs (to gauge activity and linking patterns)
  - Recent closed issues (to check label usage)

Use --project to specify which project board number to analyze.
Find project numbers with: gh velocity config discover -R owner/repo

The output is a recommended .gh-velocity.yml with comments explaining
each choice. Use --write to save it directly.`,
		Example: `  # Analyze repo and a specific project board
  gh velocity config preflight -R owner/repo --project 3

  # Without a project board (label-based analysis only)
  gh velocity config preflight -R cli/cli

  # Write config directly to .gh-velocity.yml
  gh velocity config preflight --write --project 3

  # JSON output for tooling
  gh velocity config preflight -R owner/repo -f json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Config subcommands skip PersistentPreRunE, so resolve repo here.
			repoFlag, _ := cmd.Root().PersistentFlags().GetString("repo")
			owner, repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}

			client, err := gh.NewClient(owner, repo)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			w := cmd.OutOrStdout()
			formatFlag, _ := cmd.Root().PersistentFlags().GetString("format")

			log.Notice("Analyzing %s/%s...", owner, repo)

			result, err := runPreflight(ctx, client, owner, repo, projectFlag)
			if err != nil {
				return err
			}

			if formatFlag == "json" {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			configYAML := renderPreflightConfig(result)

			// Round-trip validate: the YAML we generate must parse cleanly.
			// Suppress warnings during validation (defaults may overlap with categories).
			origWarn := config.WarnFunc
			config.WarnFunc = func(string, ...any) {} // suppress
			_, parseErr := config.Parse([]byte(configYAML))
			config.WarnFunc = origWarn
			if parseErr != nil {
				return fmt.Errorf("preflight generated invalid config (please report this): %w", parseErr)
			}

			if writeFlag {
				path := ".gh-velocity.yml"
				if _, statErr := os.Stat(path); statErr == nil {
					return &model.AppError{
						Code:    model.ErrConfigInvalid,
						Message: fmt.Sprintf("%s already exists; remove it first or edit it directly", path),
					}
				}
				if writeErr := os.WriteFile(path, []byte(configYAML), 0644); writeErr != nil {
					return fmt.Errorf("write %s: %w", path, writeErr)
				}
				log.Notice("Wrote %s", path)
				return nil
			}

			fmt.Fprint(w, configYAML)
			log.Notice("")
			log.Notice("To save this config:  gh velocity config preflight --write")
			return nil
		},
	}

	cmd.Flags().BoolVar(&writeFlag, "write", false, "Write the recommended config to .gh-velocity.yml")
	cmd.Flags().IntVar(&projectFlag, "project", 0, "Project board number to analyze (find with 'config discover')")
	return cmd
}

// PreflightResult holds the analysis of a repository for config generation.
type PreflightResult struct {
	Repo             string              `json:"repo"`
	Categories       map[string][]string `json:"categories,omitempty"`
	ActiveLabels     []string            `json:"active_labels"`
	BacklogLabels    []string            `json:"backlog_labels"`
	ProjectURL       string              `json:"project_url,omitempty"`
	StatusOptions    []string            `json:"status_options,omitempty"`
	Strategy         string              `json:"strategy"`
	HasProject       bool                `json:"has_project"`
	RecentIssues     int                 `json:"recent_issues_closed"`
	RecentPRs        int                 `json:"recent_prs_merged"`
	AllLabels        []labelCount        `json:"labels"`
	PostingReadiness *PostingReadiness   `json:"posting_readiness,omitempty"`
	Verification     *VerificationResult `json:"verification,omitempty"`
	Hints            []string            `json:"hints"`
}

// VerificationResult validates the generated config is usable.
type VerificationResult struct {
	Valid         bool     `json:"valid"`
	ConfigParses  bool     `json:"config_parses"`
	MatchersValid bool     `json:"matchers_valid"`
	CategoryCount int      `json:"category_count"`
	MissingLabels []string `json:"missing_labels,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

// PostingReadiness reports whether the token and repo support --post operations.
type PostingReadiness struct {
	DiscussionsEnabled bool     `json:"discussions_enabled"`
	HasRepoScope       bool     `json:"has_repo_scope"`
	HasProjectScope    bool     `json:"has_project_scope"`
	TokenScopes        []string `json:"token_scopes,omitempty"`   // empty for fine-grained PATs
	CategoryValid      *bool    `json:"category_valid,omitempty"` // nil = not configured
}

type labelCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func runPreflight(ctx context.Context, client *gh.Client, owner, repo string, projectNumber int) (*PreflightResult, error) {
	result := &PreflightResult{
		Repo:     owner + "/" + repo,
		Strategy: "issue", // default
	}

	// 0. Verify repo exists and normalize casing.
	canonOwner, canonRepo, exists, err := client.CanonicalRepo(ctx)
	if err != nil {
		return nil, &model.AppError{
			Code:    model.ErrNotFound,
			Message: fmt.Sprintf("could not access repository %s/%s: %v", owner, repo, err),
		}
	}
	if !exists {
		return nil, &model.AppError{
			Code:    model.ErrNotFound,
			Message: fmt.Sprintf("repository %s/%s not found — check the owner and name (case-sensitive)", owner, repo),
		}
	}
	// Use canonical casing for consistent scope queries.
	owner = canonOwner
	repo = canonRepo
	result.Repo = owner + "/" + repo

	// 1. Fetch labels from repo
	labels, err := client.ListLabels(ctx)
	if err != nil {
		result.Hints = append(result.Hints, fmt.Sprintf("Could not fetch labels: %v", err))
	} else {
		classifyLabels(result, labels)
	}

	// 2. Discover project board (if --project specified)
	if projectNumber > 0 {
		project, projErr := client.DiscoverProjectByNumber(ctx, projectNumber)
		if projErr != nil {
			result.Hints = append(result.Hints, fmt.Sprintf("Could not fetch project #%d: %v", projectNumber, projErr))
		} else {
			for _, f := range project.Fields {
				if strings.EqualFold(f.Name, "Status") && len(f.Options) > 0 {
					result.HasProject = true
					result.ProjectURL = project.URL
					for _, o := range f.Options {
						result.StatusOptions = append(result.StatusOptions, o.Name)
					}
					result.Strategy = "project-board"
					result.Hints = append(result.Hints, fmt.Sprintf("Using project board: %s (#%d)", project.Title, project.Number))
					break
				}
			}
			if !result.HasProject {
				result.Hints = append(result.Hints, fmt.Sprintf("Project #%d has no Status field with options", projectNumber))
			}
		}
	}

	// 3. Check recent activity (last 30 days)
	now := time.Now().UTC()
	since := now.Add(-30 * 24 * time.Hour)

	issues, err := client.SearchClosedIssues(ctx, since, now)
	if err != nil {
		result.Hints = append(result.Hints, fmt.Sprintf("Could not search recent issues: %v", err))
	} else {
		result.RecentIssues = len(issues)
		countLabelUsage(result, issues)
	}

	prs, err := client.SearchMergedPRs(ctx, since, now)
	if err != nil {
		result.Hints = append(result.Hints, fmt.Sprintf("Could not search recent PRs: %v", err))
	} else {
		result.RecentPRs = len(prs)
	}

	// 4. Infer strategy
	if !result.HasProject && result.RecentPRs > result.RecentIssues {
		result.Strategy = "pr"
		result.Hints = append(result.Hints, "More PRs than issues in the last 30 days — PR-centric workflow detected")
	} else if result.HasProject {
		result.Hints = append(result.Hints, "Project board detected — using project-board strategy for cycle time")
	} else {
		result.Hints = append(result.Hints, "Using default issue strategy (created → closed)")
	}

	// 5. Suggest active/backlog labels if no project board
	if !result.HasProject && len(result.ActiveLabels) > 0 {
		result.Hints = append(result.Hints, fmt.Sprintf("Found status labels: %v — can be used for WIP tracking", result.ActiveLabels))
	}

	if result.RecentIssues == 0 && result.RecentPRs == 0 {
		result.Hints = append(result.Hints, "No recent activity found in the last 30 days — metrics may be empty initially")
	}

	// 6. Check posting readiness (best-effort)
	pr := checkPostingReadiness(ctx, client)
	result.PostingReadiness = pr
	if pr.DiscussionsEnabled {
		result.Hints = append(result.Hints, "Discussions are enabled — --post can create Discussion posts")
	} else {
		result.Hints = append(result.Hints, "Discussions are disabled — enable them in repo settings for bulk --post")
	}
	if len(pr.TokenScopes) > 0 {
		result.Hints = append(result.Hints, fmt.Sprintf("Token scopes: %s", strings.Join(pr.TokenScopes, ", ")))
		if !pr.HasRepoScope {
			result.Hints = append(result.Hints, "Missing 'repo' scope — --post to issues/PRs and discussions requires 'repo'")
		}
		if !pr.HasProjectScope {
			result.Hints = append(result.Hints, "Missing 'project' scope — project board queries require 'project'")
		}
	} else {
		result.Hints = append(result.Hints, "Fine-grained PAT detected (no OAuth scopes) — ensure repo read/write and project permissions are configured")
	}

	// 7. Verify the generated config
	result.Verification = verifyConfig(result, labels)
	if v := result.Verification; v != nil {
		if v.ConfigParses {
			result.Hints = append(result.Hints, "Config verification: YAML parses cleanly")
		}
		if v.MatchersValid {
			result.Hints = append(result.Hints, fmt.Sprintf("Config verification: %d categories defined, all matchers valid", v.CategoryCount))
		}
		for _, ml := range v.MissingLabels {
			result.Hints = append(result.Hints, fmt.Sprintf("Config verification: label %q not found on repo — will never match", ml))
		}
	}

	return result, nil
}

// checkPostingReadiness probes the repo to determine posting prerequisites.
func checkPostingReadiness(ctx context.Context, client *gh.Client) *PostingReadiness {
	pr := &PostingReadiness{}

	// Check discussions enabled
	enabled, err := client.CheckDiscussionsEnabled(ctx)
	if err == nil {
		pr.DiscussionsEnabled = enabled
	}

	// Check token scopes via X-OAuth-Scopes header.
	// Fine-grained PATs return empty scopes (they use permissions, not OAuth scopes).
	scopes, err := client.TokenScopes(ctx)
	if err == nil && len(scopes) > 0 {
		pr.TokenScopes = scopes
		for _, s := range scopes {
			switch s {
			case "repo":
				pr.HasRepoScope = true
			case "project":
				pr.HasProjectScope = true
			}
		}
	}

	return pr
}

// categoryPatterns maps quality category names to the patterns used for word-boundary matching.
var categoryPatterns = map[string][]string{
	"bug":     {"bug", "defect", "regression", "crash"},
	"feature": {"enhancement", "feature", "improvement"},
	"chore":   {"chore", "maintenance", "housekeeping", "cleanup", "tech-debt", "refactor"},
	"docs":    {"documentation", "docs"},
}

// statusPatterns maps status buckets to their matching patterns.
var statusPatterns = map[string][]string{
	"active":  {"in-progress", "in progress", "wip"},
	"backlog": {"backlog", "icebox", "deferred", "wishlist"},
}

// classifyLabels sorts repo labels into quality categories and status buckets.
// Each label is assigned to the first matching category only (first-match-wins).
func classifyLabels(result *PreflightResult, labels []string) {
	// Category order determines priority for first-match-wins.
	categoryOrder := []string{"bug", "feature", "chore", "docs"}

	for _, label := range labels {
		lower := strings.ToLower(label)

		// Quality categories: first match wins
		for _, cat := range categoryOrder {
			if matchesWordAny(lower, categoryPatterns[cat]) {
				if result.Categories == nil {
					result.Categories = make(map[string][]string)
				}
				result.Categories[cat] = append(result.Categories[cat], label)
				break
			}
		}

		// Status labels: independent of categories
		if matchesWordAny(lower, statusPatterns["active"]) {
			result.ActiveLabels = append(result.ActiveLabels, label)
		}
		if matchesWordAny(lower, statusPatterns["backlog"]) {
			result.BacklogLabels = append(result.BacklogLabels, label)
		}
	}
}

// matchesWordAny returns true if any pattern matches label at a word boundary.
func matchesWordAny(label string, patterns []string) bool {
	for _, p := range patterns {
		if matchesWord(label, p) {
			return true
		}
	}
	return false
}

// matchesWord returns true if pattern appears in label at a word boundary.
// Word boundaries: start/end of string, hyphen, space, underscore, slash, colon.
// "bug" matches "bug", "bug-report", "type:bug" but NOT "debugging".
func matchesWord(label, pattern string) bool {
	idx := strings.Index(label, pattern)
	if idx < 0 {
		return false
	}
	// Check left boundary
	if idx > 0 {
		if !isWordBoundary(label[idx-1]) {
			return false
		}
	}
	// Check right boundary
	end := idx + len(pattern)
	if end < len(label) {
		if !isWordBoundary(label[end]) {
			return false
		}
	}
	return true
}

func isWordBoundary(c byte) bool {
	return c == '-' || c == ' ' || c == '_' || c == '/' || c == ':'
}

// countLabelUsage counts how often each label appears on recent closed issues.
func countLabelUsage(result *PreflightResult, issues []model.Issue) {
	counts := make(map[string]int)
	for _, issue := range issues {
		for _, l := range issue.Labels {
			counts[l]++
		}
	}

	var sorted []labelCount
	for name, count := range counts {
		sorted = append(sorted, labelCount{Name: name, Count: count})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Count > sorted[j].Count })

	// Keep top 20
	if len(sorted) > 20 {
		sorted = sorted[:20]
	}
	result.AllLabels = sorted
}

// renderPreflightConfig generates a commented .gh-velocity.yml from analysis results.
func renderPreflightConfig(r *PreflightResult) string {
	var b strings.Builder

	b.WriteString("# gh-velocity configuration\n")
	b.WriteString(fmt.Sprintf("# Generated by: gh velocity config preflight -R %s\n", r.Repo))
	b.WriteString(fmt.Sprintf("# Date: %s\n", time.Now().UTC().Format(time.DateOnly)))
	b.WriteString("#\n")
	for _, hint := range r.Hints {
		// Hints may contain newlines from error messages; comment each line.
		for line := range strings.SplitSeq(hint, "\n") {
			b.WriteString(fmt.Sprintf("# %s\n", line))
		}
	}
	b.WriteString("\n")

	// Scope
	b.WriteString("# Scope: which issues/PRs to analyze\n")
	b.WriteString("scope:\n")
	b.WriteString(fmt.Sprintf("  query: \"repo:%s\"\n", r.Repo))
	b.WriteString("\n")

	// Quality categories
	b.WriteString("# Issue classification\n")
	b.WriteString("quality:\n")
	categoryOrder := []string{"bug", "feature", "chore", "docs"}
	hasCategories := false
	for _, cat := range categoryOrder {
		if labels, ok := r.Categories[cat]; ok && len(labels) > 0 {
			hasCategories = true
			break
		}
	}
	if hasCategories {
		b.WriteString("  categories:\n")
		for _, cat := range categoryOrder {
			labels, ok := r.Categories[cat]
			if !ok || len(labels) == 0 {
				continue
			}
			b.WriteString(fmt.Sprintf("    - name: %s\n", cat))
			b.WriteString("      match:\n")
			for _, l := range labels {
				b.WriteString(fmt.Sprintf("        - \"label:%s\"\n", l))
			}
		}
	} else {
		// Sensible defaults when no labels were detected
		b.WriteString("  categories:\n")
		b.WriteString("    - name: bug\n")
		b.WriteString("      match:\n")
		b.WriteString("        - \"label:bug\"\n")
		b.WriteString("    - name: feature\n")
		b.WriteString("      match:\n")
		b.WriteString("        - \"label:enhancement\"\n")
	}
	b.WriteString("  hotfix_window_hours: 72\n")
	b.WriteString("\n")

	// Commit patterns
	b.WriteString("# Commit message scanning\n")
	b.WriteString("commit_ref:\n")
	b.WriteString("  patterns: [\"closes\"]\n")
	b.WriteString("\n")

	// Cycle time strategy
	b.WriteString(fmt.Sprintf("# Cycle time strategy: %s\n", r.Strategy))
	b.WriteString("cycle_time:\n")
	b.WriteString(fmt.Sprintf("  strategy: %s\n", r.Strategy))
	b.WriteString("\n")

	// Project board config (if detected)
	if r.HasProject && r.ProjectURL != "" {
		b.WriteString("# Projects v2 board (auto-detected)\n")
		b.WriteString("project:\n")
		b.WriteString(fmt.Sprintf("  url: %q\n", r.ProjectURL))
		b.WriteString(fmt.Sprintf("  status_field: %q\n", "Status"))
		b.WriteString("\n")

		// Map status options to lifecycle stages
		b.WriteString("# Lifecycle stages mapped from board columns\n")
		b.WriteString("lifecycle:\n")
		writeLifecycleMapping(&b, r.StatusOptions)
		b.WriteString("\n")
	}

	// Posting readiness (as comments — not a valid config key)
	if r.PostingReadiness != nil {
		b.WriteString("# Posting readiness (for --post flag):\n")
		b.WriteString("#\n")
		b.WriteString("# Required scopes per feature:\n")
		b.WriteString("#   --post to issue/PR comments:  repo\n")
		b.WriteString("#   --post to discussions:        repo (+ discussions enabled)\n")
		b.WriteString("#   project board queries:        project, read:org (if org project)\n")
		b.WriteString("#\n")
		b.WriteString("# GitHub Actions: set permissions in workflow YAML:\n")
		b.WriteString("#   permissions:\n")
		b.WriteString("#     issues: write        # for --post to issue comments\n")
		b.WriteString("#     pull-requests: write  # for --post to PR comments\n")
		b.WriteString("#     discussions: write    # for --post to discussions\n")
		b.WriteString("#\n")
		if r.PostingReadiness.DiscussionsEnabled {
			b.WriteString("#   discussions: enabled\n")
		} else {
			b.WriteString("#   discussions: disabled — enable in repo Settings → General → Features\n")
		}
		if len(r.PostingReadiness.TokenScopes) > 0 {
			b.WriteString(fmt.Sprintf("#   token scopes: %s\n", strings.Join(r.PostingReadiness.TokenScopes, ", ")))
			if !r.PostingReadiness.HasRepoScope {
				b.WriteString("#   ⚠ missing 'repo' scope — --post will fail\n")
			}
		} else {
			b.WriteString("#   fine-grained PAT — verify repo and project permissions are configured\n")
		}
		b.WriteString("\n")
	}

	// Activity summary as comments
	b.WriteString(fmt.Sprintf("# Recent activity (last 30 days): %d issues closed, %d PRs merged\n", r.RecentIssues, r.RecentPRs))

	if len(r.AllLabels) > 0 {
		b.WriteString("# Most-used labels on recent closed issues:\n")
		for _, lc := range r.AllLabels {
			if lc.Count > 1 {
				b.WriteString(fmt.Sprintf("#   %s (%d)\n", lc.Name, lc.Count))
			}
		}
	}

	return b.String()
}

// writeLifecycleMapping maps project board status options to lifecycle stages.
func writeLifecycleMapping(b *strings.Builder, options []string) {
	backlog := findStatus(options, "backlog", "to do", "todo", "triage", "new")
	inProgress := findStatus(options, "in progress", "doing", "active", "working")
	inReview := findStatus(options, "in review", "review", "pending review")
	done := findStatus(options, "done", "closed", "complete", "completed", "shipped")

	if backlog != "" {
		b.WriteString(fmt.Sprintf("  backlog:\n    project_status: [%q]\n", backlog))
	}
	if inProgress != "" {
		b.WriteString(fmt.Sprintf("  in-progress:\n    project_status: [%q]\n", inProgress))
	}
	if inReview != "" {
		b.WriteString(fmt.Sprintf("  in-review:\n    project_status: [%q]\n", inReview))
	}
	if done != "" {
		b.WriteString(fmt.Sprintf("  done:\n    project_status: [%q]\n", done))
	}

	// Show unmapped options as comments
	mapped := map[string]bool{backlog: true, inProgress: true, inReview: true, done: true}
	for _, o := range options {
		if !mapped[o] && o != "" {
			b.WriteString(fmt.Sprintf("  # unmapped: %q\n", o))
		}
	}
}

func findStatus(options []string, patterns ...string) string {
	for _, o := range options {
		lower := strings.ToLower(o)
		for _, p := range patterns {
			if strings.Contains(lower, p) {
				return o
			}
		}
	}
	return ""
}

// verifyConfig validates the generated config by parsing it, constructing a
// classifier, and cross-referencing category labels against the repo's actual labels.
func verifyConfig(result *PreflightResult, repoLabels []string) *VerificationResult {
	vr := &VerificationResult{}

	// 1. Parse the generated YAML
	configYAML := renderPreflightConfig(result)
	origWarn := config.WarnFunc
	config.WarnFunc = func(format string, args ...any) {
		vr.Warnings = append(vr.Warnings, fmt.Sprintf(format, args...))
	}
	cfg, parseErr := config.Parse([]byte(configYAML))
	config.WarnFunc = origWarn

	vr.ConfigParses = parseErr == nil
	if parseErr != nil {
		vr.Warnings = append(vr.Warnings, fmt.Sprintf("config parse error: %v", parseErr))
		return vr
	}

	// 2. Validate matchers compile via classifier
	_, classifyErr := classify.NewClassifier(cfg.Quality.Categories)
	vr.MatchersValid = classifyErr == nil
	if classifyErr != nil {
		vr.Warnings = append(vr.Warnings, fmt.Sprintf("invalid matchers: %v", classifyErr))
	}
	vr.CategoryCount = len(cfg.Quality.Categories)

	// 3. Validate strategy prerequisites
	if cfg.CycleTime.Strategy == "project-board" && cfg.Project.URL == "" {
		vr.Warnings = append(vr.Warnings, "strategy \"project-board\" requires project.url to be set")
	}

	// 4. Cross-reference category labels against repo labels
	if len(repoLabels) > 0 {
		repoLabelSet := make(map[string]bool, len(repoLabels))
		for _, l := range repoLabels {
			repoLabelSet[strings.ToLower(l)] = true
		}
		for _, cat := range cfg.Quality.Categories {
			for _, m := range cat.Matchers {
				prefix, value, ok := strings.Cut(m, ":")
				if ok && prefix == "label" {
					if !repoLabelSet[strings.ToLower(value)] {
						vr.MissingLabels = append(vr.MissingLabels, value)
						vr.Warnings = append(vr.Warnings,
							fmt.Sprintf("label %q in %s category not found on repo — will never match", value, cat.Name))
					}
				}
			}
		}
	}

	vr.Valid = vr.ConfigParses && vr.MatchersValid && len(vr.MissingLabels) == 0
	return vr
}
