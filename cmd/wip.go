package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/classify"
	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

// NewWIPCmd returns the wip command.
func NewWIPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wip",
		Short: "Show work in progress",
		Long: `Show items currently in progress.

Uses lifecycle.in-progress.match and lifecycle.in-review.match labels
from config to find open issues that are actively being worked on.

Use -R owner/repo to target a specific repo.`,
		Example: `  # Show WIP from configured lifecycle labels
  gh velocity status wip

  # JSON output for CI/automation
  gh velocity status wip -r json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWIP(cmd)
		},
	}

	return cmd
}

func runWIP(cmd *cobra.Command) error {
	ctx := cmd.Context()
	deps := DepsFromContext(ctx)
	if deps == nil {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "internal error: missing dependencies",
		}
	}

	cfg := deps.Config

	// Collect label matchers from lifecycle config.
	inProgressMatchers := cfg.Lifecycle.InProgress.Match
	inReviewMatchers := cfg.Lifecycle.InReview.Match

	if len(inProgressMatchers) == 0 && len(inReviewMatchers) == 0 {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "wip requires lifecycle.in-progress.match or lifecycle.in-review.match in config\n\n  To auto-detect your setup:  gh velocity config preflight -R owner/repo --write",
		}
	}

	client, err := deps.NewClient()
	if err != nil {
		return err
	}

	// Extract label names from "label:<name>" matchers.
	type labelStage struct {
		name  string
		stage string
	}
	var labels []labelStage
	for _, matcherStr := range inProgressMatchers {
		if _, parseErr := classify.ParseMatcher(matcherStr); parseErr != nil {
			continue
		}
		if strings.HasPrefix(matcherStr, "label:") {
			labels = append(labels, labelStage{
				name:  matcherStr[len("label:"):],
				stage: "In Progress",
			})
		}
	}
	for _, matcherStr := range inReviewMatchers {
		if _, parseErr := classify.ParseMatcher(matcherStr); parseErr != nil {
			continue
		}
		if strings.HasPrefix(matcherStr, "label:") {
			labels = append(labels, labelStage{
				name:  matcherStr[len("label:"):],
				stage: "In Review",
			})
		}
	}

	if len(labels) == 0 {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "wip: no label: matchers found in lifecycle.in-progress.match or lifecycle.in-review.match",
		}
	}

	// Build base scope.
	var baseParts []string
	if deps.Scope != "" {
		baseParts = append(baseParts, deps.Scope)
	} else if deps.Owner != "" && deps.Repo != "" {
		baseParts = append(baseParts, fmt.Sprintf("repo:%s/%s", deps.Owner, deps.Repo))
	}
	baseParts = append(baseParts, "is:open", "is:issue")
	baseQuery := strings.Join(baseParts, " ")

	// Search per label and deduplicate (GitHub search ANDs label: qualifiers,
	// so we need one search per WIP label to get OR semantics).
	now := deps.Now()
	seen := make(map[int]bool)
	var wipItems []model.WIPItem

	for _, ls := range labels {
		query := fmt.Sprintf("%s label:%q", baseQuery, ls.name)
		if deps.Debug {
			log.Debug("wip search query: %s", query)
		}

		issues, searchErr := client.SearchIssues(ctx, query)
		if searchErr != nil {
			deps.Warn("wip: search for label %q failed: %v", ls.name, searchErr)
			continue
		}

		for _, issue := range issues {
			if seen[issue.Number] {
				continue
			}
			seen[issue.Number] = true

			// Classify stage: check in-review first (more specific), then in-progress.
			stage := classifyWIPStage(issue.Labels, inProgressMatchers, inReviewMatchers)
			wipItems = append(wipItems, toWIPItemFromIssue(issue, stage, now))
		}
	}

	// Output.
	repo := fmt.Sprintf("%s/%s", deps.Owner, deps.Repo)
	rc := deps.RenderCtx(os.Stdout)
	prov := buildProvenance(cmd, map[string]string{"repository": repo})
	f := deps.ResultFormat()

	var renderErr error
	switch f {
	case format.JSON:
		renderErr = format.WriteWIPJSON(os.Stdout, repo, wipItems)
	case format.Markdown:
		renderErr = format.WriteWIPMarkdown(rc, repo, wipItems)
	default:
		renderErr = format.WriteWIPPretty(rc, repo, wipItems)
	}
	if renderErr != nil {
		return renderErr
	}
	writeProvenance(rc.Writer, f, prov)
	return nil
}

// classifyWIPStage determines whether an issue is "in-progress" or "in-review"
// based on its labels and the configured matchers.
func classifyWIPStage(labels []string, inProgressMatchers, inReviewMatchers []string) string {
	input := classify.Input{Labels: labels}

	// Check in-review first (more specific).
	for _, matcherStr := range inReviewMatchers {
		m, err := classify.ParseMatcher(matcherStr)
		if err != nil {
			continue
		}
		if m.Matches(input) {
			return "In Review"
		}
	}

	for _, matcherStr := range inProgressMatchers {
		m, err := classify.ParseMatcher(matcherStr)
		if err != nil {
			continue
		}
		if m.Matches(input) {
			return "In Progress"
		}
	}

	return "In Progress" // default
}

// toWIPItemFromIssue converts an Issue to a display-ready WIPItem.
func toWIPItemFromIssue(issue model.Issue, stage string, now time.Time) model.WIPItem {
	age := now.Sub(issue.CreatedAt)

	staleness := model.StalenessActive
	sinceUpdate := now.Sub(issue.UpdatedAt)
	switch {
	case sinceUpdate > 7*24*time.Hour:
		staleness = model.StalenessStale
	case sinceUpdate > 3*24*time.Hour:
		staleness = model.StalenessAging
	}

	return model.WIPItem{
		Number:    issue.Number,
		Title:     issue.Title,
		Status:    stage,
		Age:       age,
		Repo:      "",
		Kind:      "ISSUE",
		URL:       issue.URL,
		Labels:    issue.Labels,
		UpdatedAt: issue.UpdatedAt,
		Staleness: staleness,
	}
}
