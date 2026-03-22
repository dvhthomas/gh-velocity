package cmd

import (
	"context"
	"strings"

	"github.com/dvhthomas/gh-velocity/internal/model"
	wippipe "github.com/dvhthomas/gh-velocity/internal/pipeline/wip"
	"github.com/dvhthomas/gh-velocity/internal/posting"
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
	deps := DepsFromContext(cmd.Context())
	if deps == nil {
		return &model.AppError{Code: model.ErrConfigInvalid, Message: "internal error: missing dependencies"}
	}
	cfg := deps.Config

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

	p := &wippipe.Pipeline{
		Client:          client,
		Owner:           deps.Owner,
		Repo:            deps.Repo,
		LifecycleConfig: cfg.Lifecycle,
		EffortConfig:    cfg.Effort,
		WIPConfig:       cfg.WIP,
		ExcludeUsers:    cfg.ExcludeUsers,
		Scope:           deps.Scope,
		Now:             deps.Now(),
		Debug:           deps.Debug,
	}

	// Set enrichment callback for IssueType when matchers use type: prefix.
	if matchersHaveTypePrefix(cfg.Lifecycle.InProgress.Match, cfg.Lifecycle.InReview.Match) {
		p.EnrichFn = func(ctx context.Context) error {
			return client.EnrichIssueTypes(ctx, p.OpenIssues)
		}
	}

	return renderPipeline(cmd, deps, p, nil, posting.PostOptions{})
}

// matchersHaveTypePrefix returns true if any matcher string in any of the
// given slices starts with "type:". Used to gate IssueType enrichment.
func matchersHaveTypePrefix(matcherSets ...[]string) bool {
	for _, matchers := range matcherSets {
		for _, m := range matchers {
			if strings.HasPrefix(m, "type:") {
				return true
			}
		}
	}
	return false
}
