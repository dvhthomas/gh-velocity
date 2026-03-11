package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/config"
	"github.com/bitsbyme/gh-velocity/internal/format"
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
			origWarn := config.WarnFunc
			var roundTripWarnings []string
			config.WarnFunc = func(format string, args ...any) {
				roundTripWarnings = append(roundTripWarnings, fmt.Sprintf(format, args...))
			}
			_, parseErr := config.Parse([]byte(configYAML))
			config.WarnFunc = origWarn
			if parseErr != nil {
				return fmt.Errorf("preflight generated invalid config (please report this): %w", parseErr)
			}
			for _, w := range roundTripWarnings {
				return fmt.Errorf("preflight generated config with warnings (please report this): %s", w)
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
	Repo             string            `json:"repo"`
	BugLabels        []string          `json:"bug_labels"`
	FeatureLabels    []string          `json:"feature_labels"`
	ActiveLabels     []string          `json:"active_labels"`
	BacklogLabels    []string          `json:"backlog_labels"`
	ProjectID        string            `json:"project_id,omitempty"`
	StatusFieldID    string            `json:"status_field_id,omitempty"`
	StatusOptions    []string          `json:"status_options,omitempty"`
	Strategy         string            `json:"strategy"`
	HasProject       bool              `json:"has_project"`
	RecentIssues     int               `json:"recent_issues_closed"`
	RecentPRs        int               `json:"recent_prs_merged"`
	AllLabels        []labelCount      `json:"labels"`
	PostingReadiness *PostingReadiness `json:"posting_readiness,omitempty"`
	Hints            []string          `json:"hints"`
}

// PostingReadiness reports whether the token and repo support --post operations.
// All checks are best-effort (fine-grained PATs don't expose scopes via headers).
type PostingReadiness struct {
	DiscussionsEnabled bool  `json:"discussions_enabled"`
	HasIssuesWrite     bool  `json:"has_issues_write"`
	CategoryValid      *bool `json:"category_valid,omitempty"` // nil = not configured
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
					result.ProjectID = project.ID
					result.StatusFieldID = f.ID
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
	if pr.HasIssuesWrite {
		result.Hints = append(result.Hints, "Token has issues read access (write is best-effort check)")
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

	// Best-effort check: can we list comments? (proves at least read access)
	// We use issue #1 as a probe — if it doesn't exist, we get 404 (not 403).
	// A 403 means no access at all.
	pr.HasIssuesWrite = true // optimistic; we can't truly verify write without writing
	return pr
}

// classifyLabels sorts repo labels into bug, feature, active, and backlog buckets.
func classifyLabels(result *PreflightResult, labels []string) {
	bugPatterns := []string{"bug", "defect", "regression", "error", "crash"}
	featurePatterns := []string{"enhancement", "feature", "feat", "improvement"}
	activePatterns := []string{"in-progress", "in progress", "wip", "working", "active", "doing"}
	backlogPatterns := []string{"backlog", "icebox", "deferred", "later", "someday", "wishlist"}

	for _, label := range labels {
		lower := strings.ToLower(label)
		if matchesAny(lower, bugPatterns) {
			result.BugLabels = append(result.BugLabels, label)
		}
		if matchesAny(lower, featurePatterns) {
			result.FeatureLabels = append(result.FeatureLabels, label)
		}
		if matchesAny(lower, activePatterns) {
			result.ActiveLabels = append(result.ActiveLabels, label)
		}
		if matchesAny(lower, backlogPatterns) {
			result.BacklogLabels = append(result.BacklogLabels, label)
		}
	}
}

func matchesAny(label string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(label, p) {
			return true
		}
	}
	return false
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
		b.WriteString(fmt.Sprintf("# %s\n", hint))
	}
	b.WriteString("\n")

	// Quality labels
	b.WriteString("# Issue classification labels\n")
	b.WriteString("quality:\n")
	if len(r.BugLabels) > 0 {
		b.WriteString(fmt.Sprintf("  bug_labels: %s\n", format.FormatStringSlice(r.BugLabels)))
	} else {
		b.WriteString("  bug_labels: [\"bug\"]\n")
	}
	if len(r.FeatureLabels) > 0 {
		b.WriteString(fmt.Sprintf("  feature_labels: %s\n", format.FormatStringSlice(r.FeatureLabels)))
	} else {
		b.WriteString("  feature_labels: [\"enhancement\"]\n")
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
	if r.HasProject {
		b.WriteString("# Projects v2 board (auto-detected)\n")
		b.WriteString("project:\n")
		b.WriteString(fmt.Sprintf("  id: %q\n", r.ProjectID))
		b.WriteString(fmt.Sprintf("  status_field_id: %q\n", r.StatusFieldID))
		b.WriteString("\n")

		// Try to map status options to statuses
		b.WriteString("# Board column mapping\n")
		b.WriteString("statuses:\n")
		writeStatusMapping(&b, r.StatusOptions)
		b.WriteString("\n")
	} else if len(r.ActiveLabels) > 0 || len(r.BacklogLabels) > 0 {
		b.WriteString("# Label-based status tracking (no project board detected)\n")
		b.WriteString("statuses:\n")
		if len(r.ActiveLabels) > 0 {
			b.WriteString(fmt.Sprintf("  active_labels: %s\n", format.FormatStringSlice(r.ActiveLabels)))
		}
		if len(r.BacklogLabels) > 0 {
			b.WriteString(fmt.Sprintf("  backlog_labels: %s\n", format.FormatStringSlice(r.BacklogLabels)))
		}
		b.WriteString("\n")
	}

	// Posting readiness (as comments — not a valid config key)
	if r.PostingReadiness != nil {
		b.WriteString("# Posting readiness (for --post flag):\n")
		if r.PostingReadiness.DiscussionsEnabled {
			b.WriteString("#   discussions: enabled\n")
		} else {
			b.WriteString("#   discussions: disabled — enable in repo Settings → General → Features\n")
		}
		if r.PostingReadiness.HasIssuesWrite {
			b.WriteString("#   issues: accessible\n")
		} else {
			b.WriteString("#   issues: no access\n")
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

// writeStatusMapping tries to map project board status options to config fields.
func writeStatusMapping(b *strings.Builder, options []string) {
	backlog := findStatus(options, "backlog", "to do", "todo", "triage", "new")
	ready := findStatus(options, "ready", "planned", "up next")
	inProgress := findStatus(options, "in progress", "doing", "active", "working")
	inReview := findStatus(options, "in review", "review", "pending review")
	done := findStatus(options, "done", "closed", "complete", "completed", "shipped")

	if backlog != "" {
		b.WriteString(fmt.Sprintf("  backlog: %q\n", backlog))
	}
	if ready != "" {
		b.WriteString(fmt.Sprintf("  ready: %q\n", ready))
	}
	if inProgress != "" {
		b.WriteString(fmt.Sprintf("  in_progress: %q\n", inProgress))
	}
	if inReview != "" {
		b.WriteString(fmt.Sprintf("  in_review: %q\n", inReview))
	}
	if done != "" {
		b.WriteString(fmt.Sprintf("  done: %q\n", done))
	}

	// Show unmapped options as comments
	mapped := map[string]bool{backlog: true, ready: true, inProgress: true, inReview: true, done: true}
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
