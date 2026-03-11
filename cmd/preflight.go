package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/classify"
	"github.com/bitsbyme/gh-velocity/internal/config"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func newConfigPreflightCmd() *cobra.Command {
	var (
		writeFlag      bool
		projectURLFlag string
		projectFlag    int // deprecated
	)

	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Analyze a repo and recommend a .gh-velocity.yml configuration",
		Long: `Preflight queries your repository to build a smart starting configuration.

It inspects:
  - Labels (to detect bug, feature, and status labels)
  - A specific Projects v2 board (to find status fields and lifecycle mapping)
  - Recent merged PRs (to gauge activity and linking patterns)
  - Recent closed issues (to check label usage)

Use --project-url to include a project board in the analysis. Copy the URL
from your browser when viewing the project board.

The output is a recommended .gh-velocity.yml with comments explaining
each choice. Use --write to save it directly.`,
		Example: `  # Analyze repo with a project board
  gh velocity config preflight -R owner/repo --project-url https://github.com/orgs/myorg/projects/1

  # Without a project board (label-based analysis only)
  gh velocity config preflight -R cli/cli

  # Write config directly to .gh-velocity.yml
  gh velocity config preflight --write --project-url https://github.com/users/me/projects/3

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

			// Detect auto-detection from git remote.
			repoAutoDetected := isRepoAutoDetected(repoFlag)
			if repoAutoDetected {
				log.Notice("Using repo %s/%s from git remote (use --repo to override)", owner, repo)
			}

			client, err := gh.NewClient(owner, repo)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			w := cmd.OutOrStdout()
			formatFlag, _ := cmd.Root().PersistentFlags().GetString("format")

			// Resolve project number from --project-url or deprecated --project.
			projectNumber := projectFlag
			if projectURLFlag != "" {
				_, num, _, urlErr := gh.ParseProjectURL(projectURLFlag)
				if urlErr != nil {
					return &model.AppError{
						Code:    model.ErrConfigInvalid,
						Message: fmt.Sprintf("invalid --project-url: %v", urlErr),
					}
				}
				projectNumber = num
			}

			log.Notice("Analyzing %s/%s...", owner, repo)

			result, err := runPreflight(ctx, client, owner, repo, projectNumber)
			if err != nil {
				return err
			}

			if repoAutoDetected {
				result.RepoAutoDetected = true
				result.Hints = append(result.Hints,
					fmt.Sprintf("Repo %s auto-detected from git remote. Use -R owner/repo to target a different repository.", result.Repo))
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
	cmd.Flags().StringVar(&projectURLFlag, "project-url", "", "Project board URL (e.g., https://github.com/orgs/myorg/projects/1)")
	cmd.Flags().IntVar(&projectFlag, "project", 0, "Project board number (deprecated: use --project-url)")
	_ = cmd.Flags().MarkDeprecated("project", "use --project-url instead")
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
	DiscoveredTypes  []string            `json:"discovered_types,omitempty"`
	MatchEvidence    []CategoryEvidence  `json:"match_evidence,omitempty"`
	PostingReadiness *PostingReadiness   `json:"posting_readiness,omitempty"`
	Verification     *VerificationResult `json:"verification,omitempty"`
	RepoAutoDetected bool                `json:"repo_auto_detected"`
	Hints            []string            `json:"hints"`
}

// CategoryEvidence shows how well each matcher performs against recent data.
type CategoryEvidence struct {
	Category string            `json:"category"`
	Matchers []MatcherEvidence `json:"matchers"`
}

// MatcherEvidence records what a single matcher matched from recent items.
type MatcherEvidence struct {
	Matcher   string `json:"matcher"`             // e.g., "label:bug"
	Count     int    `json:"count"`               // total items matched
	Example   string `json:"example,omitempty"`   // "#42 Fix crash on startup"
	Suggested bool   `json:"suggested,omitempty"` // true if this is a probed alternative
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
	var projectID, statusFieldID string
	if projectNumber > 0 {
		project, projErr := client.DiscoverProjectByNumber(ctx, projectNumber)
		if projErr != nil {
			result.Hints = append(result.Hints, fmt.Sprintf("Could not fetch project #%d: %v", projectNumber, projErr))
		} else {
			projectID = project.ID
			for _, f := range project.Fields {
				if strings.EqualFold(f.Name, "Status") && len(f.Options) > 0 {
					result.HasProject = true
					result.ProjectURL = project.URL
					statusFieldID = f.ID
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

	// 5a. Discover issue types (concurrent when both paths available)
	var repoTypes, projectTypes []string

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	// Path 1: Repo-level issue types (always runs — -R is always resolved)
	g.Go(func() error {
		types, typesErr := client.ListIssueTypes(gCtx)
		if typesErr != nil {
			result.Hints = append(result.Hints, fmt.Sprintf("Could not fetch repo issue types: %v", typesErr))
			return nil
		}
		repoTypes = types
		return nil
	})

	// Path 2: Project item issue types (when project discovered)
	if projectID != "" {
		g.Go(func() error {
			items, itemsErr := client.ListProjectItems(gCtx, projectID, statusFieldID)
			if itemsErr != nil {
				result.Hints = append(result.Hints, fmt.Sprintf("Could not fetch project items for type discovery: %v", itemsErr))
				return nil
			}
			seen := make(map[string]bool)
			for _, item := range items {
				if item.IssueType != "" && !seen[item.IssueType] {
					seen[item.IssueType] = true
					projectTypes = append(projectTypes, item.IssueType)
				}
			}
			return nil
		})
	}

	_ = g.Wait()

	// Merge and deduplicate discovered types.
	typeSeen := make(map[string]bool)
	for _, t := range append(repoTypes, projectTypes...) {
		if !typeSeen[t] {
			typeSeen[t] = true
			result.DiscoveredTypes = append(result.DiscoveredTypes, t)
		}
	}

	if len(result.DiscoveredTypes) > 0 {
		result.Hints = append(result.Hints, fmt.Sprintf("Discovered issue types: %v", result.DiscoveredTypes))
	} else {
		// Be explicit so users know discovery ran and found nothing.
		if projectID != "" {
			result.Hints = append(result.Hints, "No issue types found on repo or project board items")
		} else {
			result.Hints = append(result.Hints, "No issue types configured on this repository")
		}
	}

	// Report unmapped types.
	mappedTypes := make(map[string]bool)
	for _, patterns := range typePatterns {
		for _, p := range patterns {
			mappedTypes[p] = true
		}
	}
	for _, t := range result.DiscoveredTypes {
		if !mappedTypes[t] {
			result.Hints = append(result.Hints,
				fmt.Sprintf("Discovered issue type %q with no category mapping — add type:%s to a category if desired", t, t))
		}
	}

	// 5b. Collect match evidence: run each matcher against recent items
	result.MatchEvidence = collectMatchEvidence(result.Categories, result.DiscoveredTypes, issues, prs)

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

// ignorePrefixes lists label prefixes that indicate non-category/non-status labels.
// Labels starting with these are skipped during classification.
var ignorePrefixes = []string{
	"event/",       // one-time events, not categories
	"do-not-merge", // merge gates, not workflow status
	"needs-",       // triage/process labels
}

// classifyLabels sorts repo labels into quality categories and status buckets.
// Each label is assigned to the first matching category only (first-match-wins).
func classifyLabels(result *PreflightResult, labels []string) {
	// Category order determines priority for first-match-wins.
	// Includes "docs" for label discovery even though it's not a baseline output category.
	categoryOrder := []string{"bug", "feature", "chore", "docs"}

	for _, label := range labels {
		lower := strings.ToLower(label)

		if hasIgnorePrefix(lower) {
			continue
		}

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

// hasIgnorePrefix returns true if label starts with any ignore prefix.
func hasIgnorePrefix(lower string) bool {
	for _, prefix := range ignorePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
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

// classifyItem is a unified representation of an issue or PR for matcher probing.
type classifyItem struct {
	number    int
	title     string
	labels    []string
	issueType string // GitHub Issue Type; empty for REST-sourced or PR items
}

// titleProbes maps category names to title regex patterns to try when labels find nothing.
// These cover conventional commit prefixes, common title patterns, and variants.
var titleProbes = map[string][]string{
	"bug":     {`title:/^fix[\(: ]/i`, `title:/^bug[\(: ]/i`, `title:/\bfix(es|ed)?\b/i`},
	"feature": {`title:/^feat[\(: ]/i`, `title:/^feature[\(: ]/i`, `title:/^add[\(: ]/i`},
	"chore":   {`title:/^chore[\(: ]/i`, `title:/^refactor[\(: ]/i`, `title:/^ci[\(: ]/i`, `title:/^build[\(: ]/i`},
	"docs":    {`title:/^docs?[\(: ]/i`},
}

// typePatterns maps quality categories to GitHub Issue Type names.
// Used to generate type: probe jobs from discovered issue types.
var typePatterns = map[string][]string{
	"bug":     {"Bug", "Defect"},
	"feature": {"Feature", "Enhancement"},
	"chore":   {"Task", "Chore", "Maintenance"},
}

// probeJob describes a single matcher probe to run concurrently.
type probeJob struct {
	category  string
	matcher   string
	suggested bool
}

// probeResult is the output of a single concurrent probe.
type probeResult struct {
	probeJob
	evidence MatcherEvidence
}

// collectMatchEvidence probes every matcher idea against recent items in parallel.
// For each category it tests all detected label matchers AND all title probes,
// then sorts by hit count so the best matchers surface first.
func collectMatchEvidence(categories map[string][]string, discoveredTypes []string, issues []model.Issue, prs []model.PR) []CategoryEvidence {
	var items []classifyItem
	for _, iss := range issues {
		items = append(items, classifyItem{number: iss.Number, title: iss.Title, labels: iss.Labels, issueType: iss.IssueType})
	}
	for _, pr := range prs {
		items = append(items, classifyItem{number: pr.Number, title: pr.Title, labels: pr.Labels})
	}

	if len(items) == 0 {
		return nil
	}

	// Build all probe jobs.
	categoryOrder := []string{"bug", "feature", "chore"}
	var jobs []probeJob
	for _, cat := range categoryOrder {
		// Label matchers from detected labels.
		if labels, ok := categories[cat]; ok {
			for _, label := range labels {
				jobs = append(jobs, probeJob{category: cat, matcher: "label:" + label})
			}
		}
		// Type matchers from discovered issue types (peers with labels, not suggested).
		if patterns, ok := typePatterns[cat]; ok {
			for _, typeName := range discoveredTypes {
				for _, pattern := range patterns {
					if typeName == pattern {
						jobs = append(jobs, probeJob{category: cat, matcher: "type:" + typeName})
					}
				}
			}
		}
		// Title probes as fallbacks.
		for _, probe := range titleProbes[cat] {
			jobs = append(jobs, probeJob{category: cat, matcher: probe, suggested: true})
		}
	}

	// Run all probes concurrently.
	results := make([]probeResult, len(jobs))
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(idx int, j probeJob) {
			defer wg.Done()
			me := probeMatcher(j.matcher, items)
			me.Suggested = j.suggested
			results[idx] = probeResult{probeJob: j, evidence: me}
		}(i, job)
	}
	wg.Wait()

	// Group by category and sort by hit count.
	catMatchers := make(map[string][]MatcherEvidence)
	for _, r := range results {
		catMatchers[r.category] = append(catMatchers[r.category], r.evidence)
	}

	var evidence []CategoryEvidence
	for _, cat := range categoryOrder {
		matchers := catMatchers[cat]
		if len(matchers) == 0 {
			continue
		}
		sort.Slice(matchers, func(i, j int) bool {
			return matchers[i].Count > matchers[j].Count
		})
		evidence = append(evidence, CategoryEvidence{Category: cat, Matchers: matchers})
	}
	return evidence
}

// probeMatcher tests a single matcher string against all items.
func probeMatcher(matcherStr string, items []classifyItem) MatcherEvidence {
	me := MatcherEvidence{Matcher: matcherStr}
	m, err := classify.ParseMatcher(matcherStr)
	if err != nil {
		return me
	}
	for _, item := range items {
		input := classify.Input{
			Labels:    item.labels,
			IssueType: item.issueType,
			Title:     item.title,
		}
		if m.Matches(input) {
			me.Count++
			if me.Example == "" {
				title := item.title
				if len(title) > 60 {
					title = title[:57] + "..."
				}
				me.Example = fmt.Sprintf("#%d %s", item.number, title)
			}
		}
	}
	return me
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
	b.WriteString("# Scope: which issues/PRs to analyze (GitHub search syntax)\n")
	b.WriteString("# Narrow with labels, assignees, milestones, etc.:\n")
	b.WriteString("#   query: \"repo:owner/repo label:team-backend\"\n")
	b.WriteString("#   query: \"org:myorg label:bug\"  # org-wide\n")
	b.WriteString("# Override per-run with: --scope 'label:\"priority:high\"'\n")
	b.WriteString("scope:\n")
	b.WriteString(fmt.Sprintf("  query: \"repo:%s\"\n", r.Repo))
	b.WriteString("\n")

	// Quality categories
	b.WriteString("# Issue/PR classification — assigns each item to a category.\n")
	b.WriteString("# Each entry in 'match' is a matcher string. Supported formats:\n")
	b.WriteString("#   label:<name>          — matches a GitHub label (case-insensitive)\n")
	b.WriteString("#   type:<name>           — matches a GitHub Issue Type (exact)\n")
	b.WriteString("#   title:/<regex>/       — matches the title with a Go regex\n")
	b.WriteString("#   title:/<regex>/i      — same, but case-insensitive\n")
	b.WriteString("# First category to match wins. Unmatched items are classified as \"other\".\n")
	b.WriteString("quality:\n")

	// Build effective matchers per category from evidence.
	// Use matchers with the highest hit rates.
	evidenceByCategory := make(map[string]CategoryEvidence)
	for _, ce := range r.MatchEvidence {
		evidenceByCategory[ce.Category] = ce
	}

	categoryOrder := []string{"bug", "feature", "chore"}
	type effectiveCategory struct {
		name     string
		matchers []MatcherEvidence
	}
	var cats []effectiveCategory
	for _, cat := range categoryOrder {
		ce, hasEvidence := evidenceByCategory[cat]
		if !hasEvidence {
			// No evidence data (no recent items) — use label matchers as-is.
			if labels, ok := r.Categories[cat]; ok && len(labels) > 0 {
				var mes []MatcherEvidence
				for _, l := range labels {
					mes = append(mes, MatcherEvidence{Matcher: "label:" + l})
				}
				cats = append(cats, effectiveCategory{name: cat, matchers: mes})
			}
			continue
		}

		// Separate label matchers from title probes.
		var labelHits, titleHits []MatcherEvidence
		for _, me := range ce.Matchers {
			if me.Count == 0 {
				continue
			}
			if me.Suggested {
				titleHits = append(titleHits, me)
			} else {
				labelHits = append(labelHits, me)
			}
		}

		// Prefer label matchers when they have signal.
		// Only use title matchers when labels found nothing.
		if len(labelHits) > 0 {
			cats = append(cats, effectiveCategory{name: cat, matchers: labelHits})
		} else if len(titleHits) > 0 {
			cats = append(cats, effectiveCategory{name: cat, matchers: titleHits})
		}
	}

	if len(cats) > 0 {
		b.WriteString("  categories:\n")
		for _, cat := range cats {
			b.WriteString(fmt.Sprintf("    - name: %s\n", cat.name))
			b.WriteString("      match:\n")
			for _, m := range cat.matchers {
				b.WriteString(fmt.Sprintf("        - %q\n", m.Matcher))
			}
		}
	} else {
		// Sensible defaults when nothing was detected or matched
		b.WriteString("  categories:\n")
		b.WriteString("    - name: bug\n")
		b.WriteString("      match:\n")
		b.WriteString("        - \"label:bug\"\n")
		b.WriteString("    - name: feature\n")
		b.WriteString("      match:\n")
		b.WriteString("        - \"label:enhancement\"\n")
		b.WriteString("    - name: chore\n")
		b.WriteString("      match:\n")
		b.WriteString("        - \"label:chore\"\n")
	}
	b.WriteString("  hotfix_window_hours: 72\n")

	// Match evidence: show what each matcher found in recent items.
	if len(r.MatchEvidence) > 0 {
		b.WriteString("\n")
		b.WriteString("# Match evidence (last 30 days of issues + PRs):\n")
		hasAnyMatches := false
		for _, ce := range r.MatchEvidence {
			for _, me := range ce.Matchers {
				hasAnyMatches = hasAnyMatches || me.Count > 0
				if me.Count > 0 {
					b.WriteString(fmt.Sprintf("#   %s / %s — %d matches, e.g. %s\n", ce.Category, me.Matcher, me.Count, me.Example))
				} else {
					b.WriteString(fmt.Sprintf("#   %s / %s — 0 matches (review this matcher)\n", ce.Category, me.Matcher))
				}
			}
		}
		if !hasAnyMatches {
			b.WriteString("#   (no matches found — labels may not be applied to recent items)\n")
		}
	}
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
	} else {
		// Default lifecycle without a project board
		b.WriteString("# Lifecycle: how commands identify work stages\n")
		b.WriteString("# Each stage has a query (GitHub search qualifiers) that commands append.\n")
		b.WriteString("# With a project board, add project_status arrays for board-based filtering.\n")
		b.WriteString("lifecycle:\n")
		b.WriteString("  done:\n")
		b.WriteString("    query: \"is:closed\"\n")
		if len(r.BacklogLabels) > 0 {
			b.WriteString("  backlog:\n")
			b.WriteString("    query: \"is:open")
			for _, l := range r.BacklogLabels {
				// Labels with spaces need quoting in GitHub search syntax.
				if strings.Contains(l, " ") {
					b.WriteString(fmt.Sprintf(" label:\\\"%s\\\"", l))
				} else {
					b.WriteString(fmt.Sprintf(" label:%s", l))
				}
			}
			b.WriteString("\"\n")
		}
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

	// 5. Cross-reference type: matchers against discovered types
	if len(result.DiscoveredTypes) > 0 {
		typeSet := make(map[string]bool, len(result.DiscoveredTypes))
		for _, t := range result.DiscoveredTypes {
			typeSet[t] = true
		}
		for _, cat := range cfg.Quality.Categories {
			for _, m := range cat.Matchers {
				prefix, value, ok := strings.Cut(m, ":")
				if ok && prefix == "type" {
					if !typeSet[value] {
						vr.Warnings = append(vr.Warnings,
							fmt.Sprintf("issue type %q in %s category not found on repo — will never match", value, cat.Name))
					}
				}
			}
		}
	}

	vr.Valid = vr.ConfigParses && vr.MatchersValid && len(vr.MissingLabels) == 0
	return vr
}
