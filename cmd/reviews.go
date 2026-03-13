package cmd

import (
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/pipeline"
	"github.com/bitsbyme/gh-velocity/internal/pipeline/reviews"
	"github.com/spf13/cobra"
)

// NewReviewsCmd returns the reviews command.
func NewReviewsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reviews",
		Short: "Show PRs awaiting review",
		Long: `Show open pull requests that are waiting for code review.

PRs waiting more than 48 hours are flagged as STALE.

This command shows the work (PRs), not individual reviewers.`,
		Example: `  # Show review queue
  gh velocity status reviews

  # Markdown for posting to a discussion
  gh velocity status reviews -f markdown

  # JSON for automation
  gh velocity status reviews -f json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReviews(cmd)
		},
	}

	return cmd
}

func runReviews(cmd *cobra.Command) error {
	ctx := cmd.Context()
	deps := DepsFromContext(ctx)
	if deps == nil {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "internal error: missing dependencies",
		}
	}

	client, err := deps.NewClient()
	if err != nil {
		return err
	}

	p := &reviews.Pipeline{
		Client: client,
		Owner:  deps.Owner,
		Repo:   deps.Repo,
		Now:    deps.Now(),
	}

	rc := deps.RenderCtx(cmd.OutOrStdout())
	return pipeline.RunPipeline(ctx, p, rc)
}
