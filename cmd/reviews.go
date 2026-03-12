package cmd

import (
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
)

// reviewStaleThreshold is the duration after which a PR awaiting review
// is considered stale. Hardcoded — add a flag only if users need overrides.
const reviewStaleThreshold = 48 * time.Hour

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

	client, err := gh.NewClient(deps.Owner, deps.Repo)
	if err != nil {
		return err
	}

	now := deps.Now()

	prs, err := client.SearchOpenPRsAwaitingReview(ctx)
	if err != nil {
		return &model.AppError{
			Code:    model.ErrRateLimited,
			Message: "failed to search for PRs awaiting review: " + err.Error(),
		}
	}

	repo := deps.Owner + "/" + deps.Repo
	result := model.ReviewPressureResult{
		Repository: repo,
	}

	for _, pr := range prs {
		age := now.Sub(pr.CreatedAt)
		result.AwaitingReview = append(result.AwaitingReview, model.PRAwaitingReview{
			Number:  pr.Number,
			Title:   pr.Title,
			URL:     pr.URL,
			Age:     age,
			IsStale: age > reviewStaleThreshold,
		})
	}

	// Build a search URL for open PRs awaiting review in this repo.
	searchURL := scope.OpenPRsAwaitingReviewSearchURL(deps.Owner, deps.Repo)

	w := cmd.OutOrStdout()
	rc := deps.RenderCtx(w)

	switch deps.Format {
	case format.JSON:
		return format.WriteReviewsJSON(w, result, searchURL)
	case format.Markdown:
		return format.WriteReviewsMarkdown(rc, result, searchURL)
	default:
		return format.WriteReviewsPretty(rc, result, searchURL)
	}
}
