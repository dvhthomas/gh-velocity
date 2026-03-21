package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/dvhthomas/gh-velocity/internal/config"
	"github.com/dvhthomas/gh-velocity/internal/dateutil"
	"github.com/dvhthomas/gh-velocity/internal/format"
	gh "github.com/dvhthomas/gh-velocity/internal/github"
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
		Long: `Shows what you shipped, what's blocked, and what's ahead — designed for 1:1 prep.

Sections (in order):
  Insights      Shipping velocity, AI-assisted %, lead time median & p90
  Waiting on    PRs waiting for first review, stale issues
  What I shipped Issues closed, PRs merged, PRs reviewed, releases
  What's ahead  Open issues and PRs with status annotations
  Review queue  PRs from others waiting on your review

AI-assisted PRs are tagged [ai] based on Co-Authored-By trailers and
tool badges in the PR body.

By default shows ALL your activity across repositories. Use -R to limit
to a single repo (also enables releases). Uses the authenticated GitHub
user (gh auth status).

Works without a config file or repo context — just run it from anywhere.`,
		Example: `  # All your activity in the last 7 days
  gh velocity status my-week

  # Limit to a specific repo
  gh velocity status my-week -R owner/repo

  # Last 14 days
  gh velocity status my-week --since 14d

  # Markdown for pasting into a doc
  gh velocity status my-week --results markdown`,
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
	now := nowFunc()()

	since, err := dateutil.Parse(sinceStr, now)
	if err != nil {
		return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
	}

	// Reject flags that my-week does not support (these are validated
	// by PersistentPreRunE for other commands, but my-week skips it).
	if flagBool(cmd, "post") || flagBool(cmd, "new-post") {
		return &model.AppError{Code: model.ErrConfigInvalid, Message: "my-week does not support --post"}
	}
	if flagString(cmd, "write-to") != "" {
		return &model.AppError{Code: model.ErrConfigInvalid, Message: "my-week does not support --write-to"}
	}

	// Read persistent flags from the root command.
	debugFlag := flagBool(cmd, "debug")
	noCacheFlag := flagBool(cmd, "no-cache")
	resultsFlag, _ := cmd.Flags().GetStringSlice("results")
	results, err := format.ParseResults(resultsFlag)
	if err != nil {
		return err
	}
	resultFmt := format.Pretty
	if len(results) > 0 {
		resultFmt = results[0]
	}
	suppressWarn := len(results) == 1 && results[0] == format.JSON

	// Optional config: load if present, nil if absent.
	configPath := config.DefaultConfigFile
	if f := cmd.Flag("config"); f != nil && f.Changed {
		configPath = f.Value.String()
	}
	var cfg *config.Config
	if _, statErr := os.Stat(configPath); statErr == nil {
		cfg, err = config.Load(configPath)
		if err != nil {
			return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
		}
	}

	// Optional repo: only resolve when -R or GH_REPO is set.
	// Skip git remote detection (50-200ms subprocess) in cross-repo mode.
	repoFlag := flagString(cmd, "repo")
	var owner, repo string
	var hasRepo bool
	if repoFlag != "" || os.Getenv("GH_REPO") != "" {
		o, r, err := resolveRepo(repoFlag)
		if err != nil {
			return err
		}
		owner, repo, hasRepo = o, r, true
	}

	// Build scope: merge config scope + --scope flag.
	var repoScope string
	if cfg != nil {
		repoScope = cfg.Scope.Query
	}
	scopeFlag := flagString(cmd, "scope")
	repoScope = scope.MergeScope(repoScope, scopeFlag)

	// If -R was explicitly passed but no config scope covers it, inject repo: qualifier.
	if hasRepo && repoScope == "" && repoFlag != "" {
		repoScope = fmt.Sprintf("repo:%s/%s", owner, repo)
	}

	// Exclude users from config (if available).
	var excludeUsers string
	if cfg != nil {
		excludeUsers = scope.BuildExclusions(cfg.ExcludeUsers)
	}

	// Detect terminal capabilities.
	t := term.FromEnv()
	isTTY := t.IsTerminalOutput()
	termWidth := 80
	if w, _, err := t.Size(); err == nil && w > 0 {
		termWidth = w
	}

	// Create GitHub client.
	var searchDelay time.Duration
	if cfg != nil {
		searchDelay = cfg.APIThrottleDuration()
	}
	client, err := gh.NewClient(owner, repo, searchDelay, gh.ClientOptions{NoCache: noCacheFlag})
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
		if !suppressWarn {
			log.Warn("%s", w)
		}
		warnings = append(warnings, w)
	}

	if debugFlag {
		if hasRepo {
			log.Debug("my-week repo:  %s/%s", owner, repo)
		} else {
			log.Debug("my-week repo:  (none — cross-repo mode)")
		}
		log.Debug("my-week scope: %s", repoScope)
		log.Debug("my-week user:  %s", login)
	}

	// Fetch lookback and lookahead data in parallel.
	var issuesClosed, issuesOpen []model.Issue
	var prsMerged, prsReviewed, prsOpen, prsNeedingReview, prsAwaitingMyReview []model.PR
	var releases []model.Release

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(3) // Limit concurrency to avoid GitHub secondary rate limits on search API.

	// Lookback: what happened in the --since period.
	g.Go(func() error {
		q := scope.ClosedIssuesByAuthorQuery(repoScope, login, since, now)
		q.ExcludeUsers = excludeUsers
		if debugFlag {
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
		q.ExcludeUsers = excludeUsers
		if debugFlag {
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
		q.ExcludeUsers = excludeUsers
		if debugFlag {
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
		q.ExcludeUsers = excludeUsers
		if debugFlag {
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
		q.ExcludeUsers = excludeUsers
		if debugFlag {
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
		q.ExcludeUsers = excludeUsers
		if debugFlag {
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
		q.ExcludeUsers = excludeUsers
		if debugFlag {
			log.Debug("my-week review-requested query:\n%s", q.Verbose())
		}
		prs, err := client.SearchPRs(gCtx, q.Build())
		if err != nil {
			return err
		}
		prsAwaitingMyReview = prs
		return nil
	})

	// Only fetch releases when we have a repo context.
	if hasRepo {
		g.Go(func() error {
			if debugFlag {
				log.Debug("my-week releases: listing recent releases in %s/%s", owner, repo)
			}
			rels, err := client.ListReleases(gCtx, since, now)
			if err != nil {
				// Non-fatal: some repos have no releases
				if debugFlag {
					log.Debug("my-week releases: %v (skipping)", err)
				}
				return nil
			}
			releases = rels
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// Build repo display string: "owner/repo" when scoped, empty for cross-repo.
	repoDisplay := ""
	if hasRepo {
		repoDisplay = owner + "/" + repo
	}

	result := model.MyWeekResult{
		Login:               login,
		Repo:                repoDisplay,
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

	// Compute cycle-time durations only when config provides a strategy.
	var cycleTimeDurations []time.Duration
	if cfg != nil {
		deps := &Deps{Config: cfg}
		strat := buildCycleTimeStrategy(ctx, deps, client)
		cycleTimeDurations = computeMyWeekCycleTime(ctx, strat, result)
	}

	ins := metrics.ComputeInsights(result, cycleTimeDurations)

	w := cmd.OutOrStdout()
	rc := format.RenderContext{
		Writer: w,
		Format: resultFmt,
		IsTTY:  isTTY,
		Width:  termWidth,
		Owner:  owner,
		Repo:   repo,
	}

	prov := buildProvenance(cmd, map[string]string{"login": result.Login})

	var renderErr error
	switch resultFmt {
	case format.JSON:
		renderErr = format.WriteMyWeekJSON(w, result, ins, urls, warnings)
	case format.Markdown:
		renderErr = format.WriteMyWeekMarkdown(rc, result, ins, urls)
	default:
		renderErr = format.WriteMyWeekPretty(rc, result, ins, urls)
	}
	if renderErr != nil {
		return renderErr
	}
	writeProvenance(w, resultFmt, prov)
	return nil
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
