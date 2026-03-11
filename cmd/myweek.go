package cmd

import (
	"fmt"

	"github.com/bitsbyme/gh-velocity/internal/dateutil"
	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// NewMyWeekCmd returns the my-week command.
func NewMyWeekCmd() *cobra.Command {
	var sinceFlag string

	cmd := &cobra.Command{
		Use:   "my-week",
		Short: "Your activity summary for 1:1 prep",
		Long: `Shows issues closed, PRs merged, and PRs reviewed by the authenticated user.

Designed for weekly 1:1 meetings — paste the markdown output into your prep doc.
Always uses the authenticated GitHub user (gh auth status).`,
		Example: `  # Last 7 days (default)
  gh velocity status my-week

  # Last 14 days
  gh velocity status my-week --since 14d

  # Markdown for pasting into a doc
  gh velocity status my-week -f markdown`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMyWeek(cmd, sinceFlag)
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "7d", "Lookback period (YYYY-MM-DD, RFC3339, or Nd relative)")
	return cmd
}

func runMyWeek(cmd *cobra.Command, sinceStr string) error {
	ctx := cmd.Context()
	deps := DepsFromContext(ctx)
	if deps == nil {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "internal error: missing dependencies",
		}
	}

	now := deps.Now()
	since, err := dateutil.Parse(sinceStr, now)
	if err != nil {
		return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
	}

	client, err := gh.NewClient(deps.Owner, deps.Repo)
	if err != nil {
		return err
	}

	login, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return &model.AppError{
			Code:    model.ErrAuthMissingScope,
			Message: "could not determine authenticated user: " + err.Error(),
		}
	}

	// Warn if the authenticated user looks like a bot.
	if isBotLogin(login) {
		log.Warn("Authenticated as %s — my-week shows activity for the authenticated user", login)
	}

	// my-week always targets the -R repo, ignoring config scope.
	// This is a personal activity command — you want "what did I do in repo X",
	// not "what did I do filtered by the config's scope query".
	repoScope := fmt.Sprintf("repo:%s/%s", deps.Owner, deps.Repo)

	// Fetch issues closed, PRs merged, and PRs reviewed in parallel.
	var issuesClosed []model.Issue
	var prsMerged, prsReviewed []model.PR

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	g.Go(func() error {
		q := scope.ClosedIssuesByAuthorQuery(repoScope, login, since, now)
		q.ExcludeUsers = deps.ExcludeUsers
		if deps.Debug {
			log.Debug("my-week issues query:\n%s", q.Verbose())
		}
		issues, err := client.SearchIssues(gCtx, q.Build())
		if err != nil {
			return err
		}
		issuesClosed = issues
		return nil
	})

	g.Go(func() error {
		q := scope.MergedPRsByAuthorQuery(repoScope, login, since, now)
		q.ExcludeUsers = deps.ExcludeUsers
		if deps.Debug {
			log.Debug("my-week PRs query:\n%s", q.Verbose())
		}
		prs, err := client.SearchPRs(gCtx, q.Build())
		if err != nil {
			return err
		}
		prsMerged = prs
		return nil
	})

	g.Go(func() error {
		q := scope.ReviewedPRsByAuthorQuery(repoScope, login, since, now)
		q.ExcludeUsers = deps.ExcludeUsers
		if deps.Debug {
			log.Debug("my-week reviews query:\n%s", q.Verbose())
		}
		prs, err := client.SearchPRs(gCtx, q.Build())
		if err != nil {
			return err
		}
		prsReviewed = prs
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	result := model.MyWeekResult{
		Login:        login,
		Repo:         deps.Owner + "/" + deps.Repo,
		Since:        since,
		Until:        now,
		IssuesClosed: issuesClosed,
		PRsMerged:    prsMerged,
		PRsReviewed:  prsReviewed,
	}

	w := cmd.OutOrStdout()
	rc := deps.RenderCtx(w)

	switch deps.Format {
	case format.JSON:
		return format.WriteMyWeekJSON(w, result)
	case format.Markdown:
		return format.WriteMyWeekMarkdown(rc, result)
	default:
		return format.WriteMyWeekPretty(rc, result)
	}
}

// isBotLogin returns true if the login looks like a GitHub bot account.
func isBotLogin(login string) bool {
	return len(login) > 5 && login[len(login)-5:] == "[bot]"
}
