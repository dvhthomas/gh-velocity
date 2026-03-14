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
		writeFlag      string
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

  # Write config to a custom path
  gh velocity config preflight -R cli/cli --write=output/configs/cli-cli.yml

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

			client, err := gh.NewClient(owner, repo, 0)
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

			if writeFlag != "" {
				path := writeFlag
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

			// Print diagnostic summary to stderr (not included in --write output).
			log.Notice("")
			printPreflightDiagnostics(result)
			log.Notice("")
			log.Notice("To save this config:  gh velocity config preflight --write")
			return nil
		},
	}

	cmd.Flags().StringVar(&writeFlag, "write", "", "Write config to path (default: .gh-velocity.yml)")
	cmd.Flag("write").NoOptDefVal = ".gh-velocity.yml"
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

	// Velocity heuristics
	VelocityHeuristic *VelocityHeuristic `json:"velocity_heuristic,omitempty"`
}

// VelocityHeuristic holds detected signals for velocity configuration.
type VelocityHeuristic struct {
	EffortStrategy    string              `json:"effort_strategy"`              // "numeric", "attribute", or "count"
	IterationStrategy string              `json:"iteration_strategy"`           // "project-field" or "fixed"
	NumericField      string              `json:"numeric_field,omitempty"`      // project Number field name
	IterationField    string              `json:"iteration_field,omitempty"`    // project Iteration field name
	SizingLabels      []SizingLabelMatch  `json:"sizing_labels,omitempty"`     // detected sizing labels
	Evidence          []string            `json:"evidence,omitempty"`           // human-readable evidence
}

// SizingLabelMatch maps a detected sizing label to a fibonacci effort value.
type SizingLabelMatch struct {
	Label string  `json:"label"`
	Query string  `json:"query"` // "label:size/XS"
	Value float64 `json:"value"` // fibonacci default
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
		Strategy: model.StrategyIssue, // default
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
	var projectFields []gh.DiscoveredField
	if projectNumber > 0 {
		project, projErr := client.DiscoverProjectByNumber(ctx, projectNumber)
		if projErr != nil {
			result.Hints = append(result.Hints, fmt.Sprintf("Could not fetch project #%d: %v", projectNumber, projErr))
		} else {
			projectID = project.ID
			projectFields = project.Fields
			for _, f := range project.Fields {
				if strings.EqualFold(f.Name, "Status") && len(f.Options) > 0 {
					result.HasProject = true
					result.ProjectURL = project.URL
					statusFieldID = f.ID
					for _, o := range f.Options {
						result.StatusOptions = append(result.StatusOptions, o.Name)
					}
					result.Strategy = model.StrategyIssue
					result.Hints = append(result.Hints, fmt.Sprintf("Using project board: %s (#%d) — issue strategy will use lifecycle config for cycle time", project.Title, project.Number))
					break
				}
			}
			if !result.HasProject {
				result.Hints = append(result.Hints, fmt.Sprintf("Project #%d has no Status field with options", projectNumber))
			}
		}
	}

	// 3. Check recent activity (last 30 days)
	now := nowFunc()()
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
	if result.HasProject {
		result.Strategy = model.StrategyIssue
		result.Hints = append(result.Hints, "Project board detected — issue strategy uses board status to detect work started")
	} else if len(result.ActiveLabels) > 0 {
		result.Strategy = model.StrategyIssue
		result.Hints = append(result.Hints, fmt.Sprintf("Found status labels %v — issue strategy uses label timeline for cycle time", result.ActiveLabels))
	} else if result.RecentPRs > 0 {
		result.Strategy = model.StrategyPR
		result.Hints = append(result.Hints, "No project board or status labels — using PR strategy (PR created → merged) for cycle time")
	} else {
		result.Strategy = model.StrategyIssue
		result.Hints = append(result.Hints, "No project board, status labels, or recent PRs — cycle time will be unavailable. Add a project board with: preflight --project-url <url>")
	}

	if result.RecentIssues == 0 && result.RecentPRs == 0 {
		result.Hints = append(result.Hints, "No recent activity found in the last 30 days — metrics may be empty initially")
	}

	// 5a. Discover issue types (concurrent when both paths available)
	var repoTypes, projectTypes []string

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(3) // Limit concurrency to avoid GitHub secondary rate limits.

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

	// 5c. Velocity heuristics: detect effort and iteration signals.
	detectVelocityHeuristics(result, projectFields, labels)
	if vh := result.VelocityHeuristic; vh != nil {
		for _, ev := range vh.Evidence {
			result.Hints = append(result.Hints, fmt.Sprintf("Velocity: %s", ev))
		}
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

// sizingPatterns maps label prefixes/patterns to t-shirt size → fibonacci value.
// Order matters: more specific prefixes first.
var sizingPatterns = []struct {
	prefix string
	sizes  map[string]float64
}{
	{"size/", map[string]float64{"xs": 1, "s": 2, "m": 3, "l": 5, "xl": 8}},
	{"size-", map[string]float64{"xs": 1, "s": 2, "m": 3, "l": 5, "xl": 8}},
	{"effort:", map[string]float64{"xs": 1, "s": 2, "m": 3, "l": 5, "xl": 8}},
	{"effort/", map[string]float64{"xs": 1, "s": 2, "m": 3, "l": 5, "xl": 8}},
	{"points-", map[string]float64{"1": 1, "2": 2, "3": 3, "5": 5, "8": 8, "13": 13}},
	{"estimate-", map[string]float64{"xs": 1, "s": 2, "m": 3, "l": 5, "xl": 8}},
}

// standaloneSizes maps standalone t-shirt size labels (case-insensitive).
var standaloneSizes = map[string]float64{
	"xs": 1, "s": 2, "m": 3, "l": 5, "xl": 8,
}

// detectSizingLabels scans labels for sizing patterns.
func detectSizingLabels(labels []string) []SizingLabelMatch {
	var matches []SizingLabelMatch
	seen := make(map[string]bool)

	for _, label := range labels {
		lower := strings.ToLower(label)

		// Check prefix patterns.
		matched := false
		for _, sp := range sizingPatterns {
			if strings.HasPrefix(lower, sp.prefix) {
				suffix := lower[len(sp.prefix):]
				if val, ok := sp.sizes[suffix]; ok && !seen[lower] {
					seen[lower] = true
					matches = append(matches, SizingLabelMatch{
						Label: label,
						Query: "label:" + label,
						Value: val,
					})
					matched = true
					break
				}
			}
		}
		if matched {
			continue
		}

		// Check standalone t-shirt sizes (exact match only, skip digit-only).
		if val, ok := standaloneSizes[lower]; ok && !seen[lower] {
			seen[lower] = true
			matches = append(matches, SizingLabelMatch{
				Label: label,
				Query: "label:" + label,
				Value: val,
			})
		}
	}

	// Sort by value ascending for config readability.
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Value < matches[j].Value
	})
	return matches
}

// effortFieldNames are names that suggest a Number field is used for effort.
var effortFieldNames = []string{
	"story points", "story point",
	"points", "point",
	"estimate",
	"effort",
	"size",
	"sp",
}

// detectVelocityHeuristics analyzes labels and project fields for velocity config suggestions.
func detectVelocityHeuristics(result *PreflightResult, fields []gh.DiscoveredField, labels []string) {
	vh := &VelocityHeuristic{
		EffortStrategy:    "count",
		IterationStrategy: "fixed",
	}

	// Scan project fields for Number and Iteration fields.
	var numberFieldName, iterationFieldName string
	for _, f := range fields {
		switch f.Type {
		case "ProjectV2IterationField":
			if iterationFieldName == "" {
				iterationFieldName = f.Name
				vh.Evidence = append(vh.Evidence, fmt.Sprintf("Iteration field %q found on project board", f.Name))
			}
		case "ProjectV2Field":
			// ProjectV2Field includes Text, Number, Date — match by name heuristic.
			lower := strings.ToLower(f.Name)
			for _, effortName := range effortFieldNames {
				if strings.Contains(lower, effortName) {
					if numberFieldName == "" {
						numberFieldName = f.Name
						vh.Evidence = append(vh.Evidence, fmt.Sprintf("Number field %q matches effort pattern", f.Name))
					}
					break
				}
			}
		}
	}

	// Scan labels for sizing patterns.
	sizingLabels := detectSizingLabels(labels)
	vh.SizingLabels = sizingLabels
	if len(sizingLabels) > 0 {
		labelNames := make([]string, len(sizingLabels))
		for i, sl := range sizingLabels {
			labelNames[i] = sl.Label
		}
		vh.Evidence = append(vh.Evidence, fmt.Sprintf("Sizing labels detected: %s", strings.Join(labelNames, ", ")))
	}

	// Strategy suggestion logic per plan:
	// Number field found → numeric
	// Else sizing labels found → attribute
	// Else → count
	if numberFieldName != "" {
		vh.EffortStrategy = "numeric"
		vh.NumericField = numberFieldName
	} else if len(sizingLabels) > 0 {
		vh.EffortStrategy = "attribute"
	}

	// Iteration field found → project-field, else → fixed
	if iterationFieldName != "" {
		vh.IterationStrategy = "project-field"
		vh.IterationField = iterationFieldName
	}

	result.VelocityHeuristic = vh
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
	categoryOrder := []string{"bug", "feature", "chore", "docs"}
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
	b.WriteString(fmt.Sprintf("# Date: %s\n", nowFunc()().Format(time.DateOnly)))
	b.WriteString("\n")

	// Scope
	b.WriteString("# Scope: which issues/PRs to analyze (GitHub search syntax).\n")
	b.WriteString("# Override per-run with: --scope 'label:\"priority:high\"'\n")
	b.WriteString("scope:\n")
	b.WriteString(fmt.Sprintf("  query: \"repo:%s\"\n", r.Repo))
	b.WriteString("\n")

	// Quality categories
	b.WriteString("# Issue/PR classification — first matching category wins; unmatched = \"other\".\n")
	b.WriteString("# Matchers: label:<name>, type:<name>, title:/<regex>/i\n")
	b.WriteString("quality:\n")

	// Build effective matchers per category from evidence.
	// Use matchers with the highest hit rates.
	evidenceByCategory := make(map[string]CategoryEvidence)
	for _, ce := range r.MatchEvidence {
		evidenceByCategory[ce.Category] = ce
	}

	categoryOrder := []string{"bug", "feature", "chore", "docs"}
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
		hasTitleSignal := false
		for _, me := range ce.Matchers {
			if me.Suggested {
				if me.Count > 0 {
					hasTitleSignal = true
				}
				titleHits = append(titleHits, me)
			} else if me.Count > 0 {
				labelHits = append(labelHits, me)
			}
		}
		// Only include title probes when at least one in the category had signal.
		// This keeps all conventional-commit prefixes for the category together.
		if !hasTitleSignal {
			titleHits = nil
		}

		// Always include detected label matchers from classifyLabels,
		// even if they had 0 recent hits. Labels are the canonical
		// classification mechanism; title probes supplement them.
		if len(labelHits) == 0 {
			if labels, ok := r.Categories[cat]; ok {
				for _, l := range labels {
					labelHits = append(labelHits, MatcherEvidence{Matcher: "label:" + l})
				}
			}
		}

		// Combine: label matchers first, then title probes as fallbacks.
		var combined []MatcherEvidence
		combined = append(combined, labelHits...)
		combined = append(combined, titleHits...)
		if len(combined) > 0 {
			cats = append(cats, effectiveCategory{name: cat, matchers: combined})
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

	// Exclude users — common bots enabled by default (no-op if absent from the repo).
	b.WriteString("# Exclude bot accounts from metrics.\n")
	b.WriteString("exclude_users:\n")
	b.WriteString("  - \"dependabot[bot]\"\n")
	b.WriteString("\n")

	// API throttle — prevents GitHub secondary rate limits.
	b.WriteString("# Minimum seconds between GitHub search API calls.\n")
	b.WriteString("# Prevents secondary (abuse) rate limits which trigger on burst traffic.\n")
	b.WriteString("# Set to 0 to disable (not recommended for CI).\n")
	b.WriteString(fmt.Sprintf("api_throttle_seconds: %d\n", config.DefaultAPIThrottleSeconds))
	b.WriteString("\n")

	// Project board config (if detected)
	if r.HasProject && r.ProjectURL != "" {
		b.WriteString("# Projects v2 board (auto-detected)\n")
		b.WriteString("# CI/Actions: GITHUB_TOKEN cannot access projects. Create a classic PAT with\n")
		b.WriteString("# 'project' scope: https://github.com/settings/tokens/new?scopes=project&description=gh-velocity\n")
		b.WriteString("project:\n")
		b.WriteString(fmt.Sprintf("  url: %q\n", r.ProjectURL))
		b.WriteString(fmt.Sprintf("  status_field: %q\n", "Status"))
		b.WriteString("\n")

		// Map status options to lifecycle stages
		b.WriteString("# Lifecycle stages mapped from board columns\n")
		b.WriteString("# project_status: used for WIP and backlog detection\n")
		b.WriteString("# match: used for cycle time (label timestamps are more reliable than board timestamps)\n")
		b.WriteString("lifecycle:\n")
		writeLifecycleMapping(&b, r.StatusOptions, r.ActiveLabels)
		b.WriteString("\n")
	} else if len(r.ActiveLabels) > 0 || len(r.BacklogLabels) > 0 {
		// Label-based lifecycle (no project board).
		b.WriteString("# Lifecycle stages (label-based, no project board)\n")
		b.WriteString("# match: patterns use classify.Matcher syntax for cycle time detection\n")
		b.WriteString("lifecycle:\n")
		if len(r.BacklogLabels) > 0 {
			b.WriteString("  backlog:\n")
			b.WriteString("    match:\n")
			for _, l := range r.BacklogLabels {
				b.WriteString(fmt.Sprintf("      - \"label:%s\"\n", l))
			}
		}
		if len(r.ActiveLabels) > 0 {
			b.WriteString("  in-progress:\n")
			b.WriteString("    match:\n")
			for _, l := range r.ActiveLabels {
				b.WriteString(fmt.Sprintf("      - \"label:%s\"\n", l))
			}
		}
		b.WriteString("\n")
	} else {
		b.WriteString("# No lifecycle stages detected. Add a project board or status labels for cycle time metrics:\n")
		b.WriteString("#   gh velocity config preflight --project-url <url> --write\n")
		b.WriteString("\n")
	}

	// Discussions section: emit when detected.
	if r.PostingReadiness != nil && r.PostingReadiness.DiscussionsEnabled {
		b.WriteString("# Post bulk reports (--post) as Discussion posts.\n")
		b.WriteString("# CI/Actions: requires 'discussions: write' in workflow permissions.\n")
		b.WriteString("discussions:\n")
		b.WriteString("  category: General\n")
		b.WriteString("\n")
	}

	// Velocity section.
	renderVelocityConfig(&b, r)

	return b.String()
}

// renderVelocityConfig emits the velocity: section from heuristic results.
func renderVelocityConfig(b *strings.Builder, r *PreflightResult) {
	vh := r.VelocityHeuristic
	if vh == nil {
		// No heuristic ran — emit commented defaults.
		b.WriteString("# Velocity: effort completed per iteration.\n")
		b.WriteString("# Run: gh velocity config preflight --project-url <url> --write\n")
		b.WriteString("# velocity:\n")
		b.WriteString("#   unit: issues\n")
		b.WriteString("#   effort:\n")
		b.WriteString("#     strategy: count\n")
		b.WriteString("#   iteration:\n")
		b.WriteString("#     strategy: fixed\n")
		b.WriteString("#     fixed:\n")
		b.WriteString("#       length: \"14d\"\n")
		b.WriteString("#       anchor: \"2026-01-06\"\n")
		b.WriteString("#     count: 3\n")
		b.WriteString("\n")
		return
	}

	b.WriteString("# Velocity: effort completed per iteration.\n")
	for _, ev := range vh.Evidence {
		b.WriteString(fmt.Sprintf("# Evidence: %s\n", ev))
	}
	b.WriteString("velocity:\n")
	b.WriteString("  unit: issues\n")
	b.WriteString("  effort:\n")
	b.WriteString(fmt.Sprintf("    strategy: %s\n", vh.EffortStrategy))

	switch vh.EffortStrategy {
	case "numeric":
		b.WriteString("    numeric:\n")
		b.WriteString(fmt.Sprintf("      project_field: %q\n", vh.NumericField))
	case "attribute":
		b.WriteString("    attribute:\n")
		for _, sl := range vh.SizingLabels {
			b.WriteString(fmt.Sprintf("      - query: %q\n", sl.Query))
			b.WriteString(fmt.Sprintf("        value: %.0f\n", sl.Value))
		}
	}

	b.WriteString("  iteration:\n")
	b.WriteString(fmt.Sprintf("    strategy: %s\n", vh.IterationStrategy))

	switch vh.IterationStrategy {
	case "project-field":
		b.WriteString(fmt.Sprintf("    project_field: %q\n", vh.IterationField))
	case "fixed":
		b.WriteString("    fixed:\n")
		b.WriteString("      length: \"14d\"\n")
		now := nowFunc()()
		// Anchor on the most recent Monday.
		daysFromMonday := (int(now.Weekday()) + 6) % 7
		anchor := now.AddDate(0, 0, -daysFromMonday)
		b.WriteString(fmt.Sprintf("      anchor: %q\n", anchor.Format("2006-01-02")))
	}

	b.WriteString("    count: 3\n")
	b.WriteString("\n")
}

// printPreflightDiagnostics writes analysis details to stderr for interactive use.
// This info helps users understand what preflight found, but doesn't belong in the config file.
func printPreflightDiagnostics(r *PreflightResult) {
	// Hints (strategy choice, project board detection, warnings)
	for _, hint := range r.Hints {
		log.Notice("  %s", hint)
	}

	// Match evidence
	if len(r.MatchEvidence) > 0 {
		log.Notice("")
		log.Notice("Match evidence (last 30 days):")
		for _, ce := range r.MatchEvidence {
			for _, me := range ce.Matchers {
				if me.Count > 0 {
					log.Notice("  %s / %s — %d matches, e.g. %s", ce.Category, me.Matcher, me.Count, me.Example)
				} else {
					log.Notice("  %s / %s — 0 matches", ce.Category, me.Matcher)
				}
			}
		}
	}

	// Activity summary
	log.Notice("")
	log.Notice("Recent activity (last 30 days): %d issues closed, %d PRs merged", r.RecentIssues, r.RecentPRs)

	// Posting readiness
	if pr := r.PostingReadiness; pr != nil {
		if pr.DiscussionsEnabled {
			log.Notice("Discussions: enabled")
		} else {
			log.Notice("Discussions: disabled (enable in repo Settings → General → Features)")
		}
		if len(pr.TokenScopes) > 0 {
			log.Notice("Token scopes: %s", strings.Join(pr.TokenScopes, ", "))
			if !pr.HasRepoScope {
				log.Notice("⚠ Missing 'repo' scope — --post will fail")
			}
		}
	}

	// Verification
	if v := r.Verification; v != nil {
		if v.Valid {
			log.Notice("Config verification: OK (%d categories)", v.CategoryCount)
		} else {
			for _, w := range v.Warnings {
				log.Notice("Config verification: %s", w)
			}
		}
	}
}

// writeLifecycleMapping maps project board status options to lifecycle stages.
// When activeLabels are detected, it also emits match entries for the in-progress
// stage so cycle time can use label timestamps (more reliable than board timestamps).
func writeLifecycleMapping(b *strings.Builder, options []string, activeLabels []string) {
	backlog := findStatuses(options, "backlog", "to do", "todo", "triage", "new", "ready")
	inProgress := findStatuses(options, "in progress", "doing", "active", "working")
	inReview := findStatuses(options, "in review", "review", "pending review")
	done := findStatuses(options, "done", "closed", "complete", "completed", "shipped")

	writeStage := func(name string, statuses []string) {
		if len(statuses) == 0 {
			return
		}
		quoted := make([]string, len(statuses))
		for i, s := range statuses {
			quoted[i] = fmt.Sprintf("%q", s)
		}
		b.WriteString(fmt.Sprintf("  %s:\n    project_status: [%s]\n", name, strings.Join(quoted, ", ")))
	}
	writeStage("backlog", backlog)

	// In-progress: emit project_status + match (labels) when available.
	if len(inProgress) > 0 {
		quoted := make([]string, len(inProgress))
		for i, s := range inProgress {
			quoted[i] = fmt.Sprintf("%q", s)
		}
		b.WriteString(fmt.Sprintf("  in-progress:\n    project_status: [%s]\n", strings.Join(quoted, ", ")))
	} else {
		b.WriteString("  in-progress:\n")
	}
	if len(activeLabels) > 0 {
		b.WriteString("    match:\n")
		for _, l := range activeLabels {
			b.WriteString(fmt.Sprintf("      - \"label:%s\"\n", l))
		}
	} else if len(inProgress) > 0 {
		b.WriteString("    # Tip: add a label like \"in-progress\" for more reliable cycle time timestamps.\n")
		b.WriteString("    # Label events have immutable timestamps; board status updatedAt can be stale.\n")
	}

	writeStage("in-review", inReview)
	writeStage("done", done)

	// Show unmapped options as comments.
	mapped := make(map[string]bool)
	for _, group := range [][]string{backlog, inProgress, inReview, done} {
		for _, s := range group {
			mapped[s] = true
		}
	}
	for _, o := range options {
		if !mapped[o] && o != "" {
			b.WriteString(fmt.Sprintf("  # unmapped: %q\n", o))
		}
	}
}

// findStatuses returns all options that match any of the patterns.
func findStatuses(options []string, patterns ...string) []string {
	var result []string
	for _, o := range options {
		lower := strings.ToLower(o)
		for _, p := range patterns {
			if strings.Contains(lower, p) {
				result = append(result, o)
				break
			}
		}
	}
	return result
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
	hasProjectStatus := len(cfg.Lifecycle.InProgress.ProjectStatus) > 0
	hasMatch := len(cfg.Lifecycle.InProgress.Match) > 0
	if cfg.CycleTime.Strategy == model.StrategyIssue && !hasProjectStatus && !hasMatch {
		vr.Warnings = append(vr.Warnings, "issue strategy has no lifecycle.in-progress.project_status or match — cycle time will be unavailable; configure lifecycle.in-progress for cycle time metrics")
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
