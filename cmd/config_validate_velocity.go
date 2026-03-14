package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/classify"
	"github.com/bitsbyme/gh-velocity/internal/config"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/pipeline/velocity"
	"github.com/bitsbyme/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
)

// addVelocityValidateFlag adds the --velocity flag to config validate.
func addVelocityValidateFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("velocity", false, "Live-test velocity matchers against recent issues")
}

// runVelocityValidation executes velocity-specific config validation.
// Returns true if the --velocity flag was set and validation ran.
func runVelocityValidation(cmd *cobra.Command, cfg *config.Config) (bool, error) {
	velocityFlag, _ := cmd.Flags().GetBool("velocity")
	if !velocityFlag {
		return false, nil
	}

	if cfg.Velocity.Iteration.Strategy == "" && cfg.Velocity.Effort.Strategy == "count" {
		fmt.Fprintln(cmd.OutOrStdout(), "velocity: using defaults (count effort, no iteration strategy)")
		fmt.Fprintln(cmd.OutOrStdout(), "  hint: run 'gh velocity config preflight' to discover velocity config")
		return true, nil
	}

	// Resolve repo.
	repoFlag, _ := cmd.Root().PersistentFlags().GetString("repo")
	owner, repo, err := resolveRepo(repoFlag)
	if err != nil {
		return true, err
	}

	client, err := gh.NewClient(owner, repo, 0)
	if err != nil {
		return true, err
	}

	w := cmd.OutOrStdout()
	ctx := cmd.Context()

	switch cfg.Velocity.Effort.Strategy {
	case "attribute":
		if err := validateAttributeEffort(ctx, w, cfg, client, owner, repo); err != nil {
			return true, err
		}
	case "numeric":
		if err := validateNumericEffort(ctx, w, cfg, client); err != nil {
			return true, err
		}
	case "count":
		fmt.Fprintln(w, "velocity effort: count (every item = 1)")
	}

	if cfg.Velocity.Iteration.Strategy == "project-field" {
		if err := validateProjectFieldIteration(ctx, w, cfg, client); err != nil {
			return true, err
		}
	} else if cfg.Velocity.Iteration.Strategy == "fixed" {
		fmt.Fprintf(w, "velocity iteration: fixed (%s from %s)\n",
			cfg.Velocity.Iteration.Fixed.Length, cfg.Velocity.Iteration.Fixed.Anchor)
	}

	// Show API budget after validation.
	printAPIUsage(ctx, client)

	return true, nil
}

func validateAttributeEffort(ctx context.Context, w io.Writer, cfg *config.Config, client *gh.Client, owner, repo string) error {
	hasFieldMatchers := velocity.HasFieldMatchers(cfg.Velocity.Effort)

	if hasFieldMatchers {
		return validateAttributeEffortFromBoard(ctx, w, cfg, client)
	}
	return validateAttributeEffortFromSearch(ctx, w, cfg, client, owner, repo)
}

// validateAttributeEffortFromBoard fetches project board items with SingleSelect
// field values, then tests field: matchers against them.
func validateAttributeEffortFromBoard(ctx context.Context, w io.Writer, cfg *config.Config, client *gh.Client) error {
	fmt.Fprintln(w, "velocity effort: attribute (testing field: matchers against board items)")

	if cfg.Project.URL == "" {
		return fmt.Errorf("velocity validate: project.url required for field: matchers")
	}

	projInfo, err := client.ResolveProject(ctx, cfg.Project.URL, "")
	if err != nil {
		return &model.AppError{
			Code:    model.ErrNotFound,
			Message: fmt.Sprintf("resolve project for attribute validation: %v", err),
		}
	}

	ssFields := velocity.ExtractFieldMatcherNames(cfg.Velocity.Effort)
	items, err := client.ListProjectItemsWithFields(ctx, projInfo.ProjectID, "", "", ssFields)
	if err != nil {
		return fmt.Errorf("velocity validate: %w", err)
	}

	// Run matchers against board items.
	counts := make([]effortMatchCount, len(cfg.Velocity.Effort.Attribute))
	for i, m := range cfg.Velocity.Effort.Attribute {
		counts[i] = effortMatchCount{query: m.Query, value: m.Value}
	}

	unmatched := 0
	for _, item := range items {
		input := classify.Input{
			Labels:    item.Labels,
			IssueType: item.IssueType,
			Title:     item.Title,
			Fields:    item.Fields,
		}
		matched := false
		for i, m := range cfg.Velocity.Effort.Attribute {
			parsed, _ := classify.ParseMatcher(m.Query)
			if parsed.Matches(input) {
				counts[i].count++
				matched = true
				break
			}
		}
		if !matched {
			unmatched++
		}
	}

	printAttributeDistribution(w, counts, unmatched, len(items), "board items")
	return nil
}

// validateAttributeEffortFromSearch tests label/type/title matchers against
// recent closed issues fetched via the search API.
func validateAttributeEffortFromSearch(ctx context.Context, w io.Writer, cfg *config.Config, client *gh.Client, owner, repo string) error {
	fmt.Fprintln(w, "velocity effort: attribute (testing matchers against recent issues)")

	scopeStr := cfg.Scope.Query
	if scopeStr == "" {
		scopeStr = fmt.Sprintf("repo:%s/%s", owner, repo)
	}
	now := time.Now().UTC()
	q := scope.ClosedIssueQuery(scopeStr, now.AddDate(0, 0, -90), now)
	issues, err := client.SearchIssues(ctx, q.Build())
	if err != nil {
		return fmt.Errorf("velocity validate: %w", err)
	}

	counts := make([]effortMatchCount, len(cfg.Velocity.Effort.Attribute))
	for i, m := range cfg.Velocity.Effort.Attribute {
		counts[i] = effortMatchCount{query: m.Query, value: m.Value}
	}

	unmatched := 0
	for _, iss := range issues {
		input := classify.Input{
			Labels:    iss.Labels,
			IssueType: iss.IssueType,
			Title:     iss.Title,
		}
		matched := false
		for i, m := range cfg.Velocity.Effort.Attribute {
			parsed, _ := classify.ParseMatcher(m.Query)
			if parsed.Matches(input) {
				counts[i].count++
				matched = true
				break
			}
		}
		if !matched {
			unmatched++
		}
	}

	printAttributeDistribution(w, counts, unmatched, len(issues), "issues, last 90 days")
	return nil
}

type effortMatchCount struct {
	query string
	value float64
	count int
}

func printAttributeDistribution(w io.Writer, counts []effortMatchCount, unmatched, total int, source string) {
	fmt.Fprintf(w, "\n  Distribution (%d %s):\n", total, source)
	fmt.Fprintf(w, "  %-30s %6s %8s\n", "Matcher", "Value", "Count")
	fmt.Fprintf(w, "  %-30s %6s %8s\n", strings.Repeat("─", 30), strings.Repeat("─", 6), strings.Repeat("─", 8))
	for _, mc := range counts {
		fmt.Fprintf(w, "  %-30s %6.0f %8d\n", mc.query, mc.value, mc.count)
	}
	fmt.Fprintf(w, "  %-30s %6s %8d\n", "(not assessed)", "", unmatched)

	if unmatched > 0 && total > 0 {
		pct := float64(unmatched) / float64(total) * 100
		fmt.Fprintf(w, "\n  %.0f%% of items unmatched — consider adding more matchers\n", pct)
	}
}

func validateNumericEffort(ctx context.Context, w io.Writer, cfg *config.Config, client *gh.Client) error {
	fmt.Fprintf(w, "velocity effort: numeric (field %q)\n", cfg.Velocity.Effort.Numeric.ProjectField)

	projInfo, err := client.ResolveProject(ctx, cfg.Project.URL, "")
	if err != nil {
		return &model.AppError{
			Code:    model.ErrNotFound,
			Message: fmt.Sprintf("resolve project for numeric validation: %v", err),
		}
	}

	items, err := client.ListProjectItemsWithFields(ctx, projInfo.ProjectID, "", cfg.Velocity.Effort.Numeric.ProjectField, nil)
	if err != nil {
		return fmt.Errorf("velocity validate: %w", err)
	}

	var withValue, withoutValue, withZero, withNegative int
	for _, item := range items {
		if item.Effort == nil {
			withoutValue++
		} else if *item.Effort < 0 {
			withNegative++
		} else if *item.Effort == 0 {
			withZero++
		} else {
			withValue++
		}
	}

	fmt.Fprintf(w, "\n  Field distribution (%d items):\n", len(items))
	fmt.Fprintf(w, "    With value:    %d\n", withValue)
	fmt.Fprintf(w, "    Zero:          %d\n", withZero)
	fmt.Fprintf(w, "    Not set:       %d (not assessed)\n", withoutValue)
	if withNegative > 0 {
		fmt.Fprintf(w, "    Negative:      %d (treated as not assessed)\n", withNegative)
	}

	return nil
}

func validateProjectFieldIteration(ctx context.Context, w io.Writer, cfg *config.Config, client *gh.Client) error {
	fmt.Fprintf(w, "velocity iteration: project-field (%q)\n", cfg.Velocity.Iteration.ProjectField)

	projInfo, err := client.ResolveProject(ctx, cfg.Project.URL, "")
	if err != nil {
		return &model.AppError{
			Code:    model.ErrNotFound,
			Message: fmt.Sprintf("resolve project for iteration validation: %v", err),
		}
	}

	iterCfg, err := client.ListIterationField(ctx, projInfo.ProjectID, cfg.Velocity.Iteration.ProjectField)
	if err != nil {
		return &model.AppError{
			Code:    model.ErrNotFound,
			Message: fmt.Sprintf("iteration field %q: %v", cfg.Velocity.Iteration.ProjectField, err),
		}
	}

	fmt.Fprintf(w, "  Active iterations:    %d\n", len(iterCfg.Iterations))
	fmt.Fprintf(w, "  Completed iterations: %d\n", len(iterCfg.CompletedIterations))

	if len(iterCfg.Iterations) > 0 {
		fmt.Fprintf(w, "  Current: %s (%s – %s)\n",
			iterCfg.Iterations[0].Title,
			iterCfg.Iterations[0].StartDate.Format("2006-01-02"),
			iterCfg.Iterations[0].EndDate.Format("2006-01-02"))
	}

	return nil
}
