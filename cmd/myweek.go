package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/dateutil"
	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/scope"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// NewMyWeekCmd returns the my-week command.
func NewMyWeekCmd() *cobra.Command {
	var sinceFlag string

	cmd := &cobra.Command{
		Use:   "my-week",
		Short: "Your activity summary for 1:1 prep",
		Long: `Shows what you shipped and what's ahead — designed for 1:1 prep.

Lookback: issues closed, PRs merged, PRs reviewed in the --since period.
Lookahead: open issues assigned to you, open PRs you authored.

Always uses the authenticated GitHub user (gh auth status).`,
		Example: `  # Last 7 days (default)
  gh velocity status my-week

  # Last 14 days
  gh velocity status my-week --since 14d

  # Markdown for pasting into a doc
  gh velocity status my-week -r markdown`,
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

	client, err := deps.NewClient()
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
	var warnings []string
	if isBotLogin(login) {
		w := fmt.Sprintf("Authenticated as %s — my-week shows activity for the authenticated user", login)
		deps.Warn("%s", w)
		warnings = append(warnings, w)
	}

	// Use the resolved scope from config + --scope flag, which already
	// falls back to repo:owner/repo when no config scope is set.
	// This lets configs define cross-repo or project-board scopes.
	repoScope := deps.Scope

	// Fetch lookback and lookahead data in parallel.
	var issuesClosed, issuesOpen []model.Issue
	var prsMerged, prsReviewed, prsOpen, prsNeedingReview, prsAwaitingMyReview []model.PR
	var releases []model.Release

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(3) // Limit concurrency to avoid GitHub secondary rate limits on search API.

	// Lookback: what happened in the --since period.
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

	// Lookahead: what's in progress right now.
	g.Go(func() error {
		q := scope.OpenIssuesByAssigneeQuery(repoScope, login)
		q.ExcludeUsers = deps.ExcludeUsers
		if deps.Debug {
			log.Debug("my-week open issues query:\n%s", q.Verbose())
		}
		issues, err := client.SearchIssues(gCtx, q.Build())
		if err != nil {
			return err
		}
		issuesOpen = issues
		return nil
	})

	g.Go(func() error {
		q := scope.OpenPRsByAuthorQuery(repoScope, login)
		q.ExcludeUsers = deps.ExcludeUsers
		if deps.Debug {
			log.Debug("my-week open PRs query:\n%s", q.Verbose())
		}
		prs, err := client.SearchPRs(gCtx, q.Build())
		if err != nil {
			return err
		}
		prsOpen = prs
		return nil
	})

	g.Go(func() error {
		q := scope.OpenPRsNeedingReviewQuery(repoScope, login)
		q.ExcludeUsers = deps.ExcludeUsers
		if deps.Debug {
			log.Debug("my-week PRs needing review query:\n%s", q.Verbose())
		}
		prs, err := client.SearchPRs(gCtx, q.Build())
		if err != nil {
			return err
		}
		prsNeedingReview = prs
		return nil
	})

	g.Go(func() error {
		q := scope.ReviewRequestedQuery(repoScope, login)
		q.ExcludeUsers = deps.ExcludeUsers
		if deps.Debug {
			log.Debug("my-week review-requested query:\n%s", q.Verbose())
		}
		prs, err := client.SearchPRs(gCtx, q.Build())
		if err != nil {
			return err
		}
		prsAwaitingMyReview = prs
		return nil
	})

	g.Go(func() error {
		if deps.Debug {
			log.Debug("my-week releases: listing recent releases in %s/%s", deps.Owner, deps.Repo)
		}
		rels, err := client.ListReleases(gCtx, since, now)
		if err != nil {
			// Non-fatal: some repos have no releases
			if deps.Debug {
				log.Debug("my-week releases: %v (skipping)", err)
			}
			return nil
		}
		releases = rels
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	result := model.MyWeekResult{
		Login:               login,
		Repo:                deps.Owner + "/" + deps.Repo,
		Since:               since,
		Until:               now,
		IssuesClosed:        issuesClosed,
		PRsMerged:           prsMerged,
		PRsReviewed:         prsReviewed,
		IssuesOpen:          issuesOpen,
		PRsOpen:             prsOpen,
		PRsNeedingReview:    prsNeedingReview,
		PRsAwaitingMyReview: prsAwaitingMyReview,
		Releases:            releases,
	}

	// Compute search URLs for lookback sections.
	urls := format.MyWeekSearchURLs{
		IssuesClosed: scope.ClosedIssuesByAuthorQuery(repoScope, login, since, now).URL(),
		PRsMerged:    scope.MergedPRsByAuthorQuery(repoScope, login, since, now).URL(),
		PRsReviewed:  scope.ReviewedPRsByAuthorQuery(repoScope, login, since, now).URL(),
	}

	// Compute cycle-time durations using the configured strategy.
	strat := buildCycleTimeStrategy(ctx, deps, client)
	cycleTimeDurations := computeMyWeekCycleTime(ctx, strat, result)

	ins := metrics.ComputeInsights(result, cycleTimeDurations)

	w := cmd.OutOrStdout()
	rc := deps.RenderCtx(w)

	switch deps.ResultFormat() {
	case format.JSON:
		return format.WriteMyWeekJSON(w, result, ins, urls, warnings)
	case format.Markdown:
		return format.WriteMyWeekMarkdown(rc, result, ins, urls)
	default:
		return format.WriteMyWeekPretty(rc, result, ins, urls)
	}
}

// computeMyWeekCycleTime computes cycle-time durations for closed issues
// using the configured strategy. For issue strategy, this calls the GitHub API
// for each issue. For PR strategy, it uses PR created → merged.
// Returns nil when the strategy has no signal (e.g., no project configured).
func computeMyWeekCycleTime(ctx context.Context, strat metrics.CycleTimeStrategy, r model.MyWeekResult) []time.Duration {
	switch strat.Name() {
	case model.StrategyPR:
		// PR strategy: PR created → merged for merged PRs
		var durations []time.Duration
		for _, pr := range r.PRsMerged {
			if pr.MergedAt != nil {
				d := pr.MergedAt.Sub(pr.CreatedAt)
				if d > 0 {
					durations = append(durations, d)
				}
			}
		}
		return durations
	default: // "issue"
		// Issue strategy: use strategy.Compute for each closed issue
		var durations []time.Duration
		for i := range r.IssuesClosed {
			iss := r.IssuesClosed[i]
			input := metrics.CycleTimeInput{Issue: &iss}
			m := strat.Compute(ctx, input)
			if m.Duration != nil && *m.Duration > 0 {
				durations = append(durations, *m.Duration)
			}
		}
		return durations
	}
}

// isBotLogin returns true if the login looks like a GitHub bot account.
func isBotLogin(login string) bool {
	return len(login) > 5 && login[len(login)-5:] == "[bot]"
}
