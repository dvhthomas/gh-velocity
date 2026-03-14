package cmd

import (
	"context"
	"fmt"
	"maps"
	"os"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/classify"
	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/gitdata"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/pipeline/release"
	"github.com/bitsbyme/gh-velocity/internal/posting"
	"github.com/bitsbyme/gh-velocity/internal/strategy"
	"github.com/spf13/cobra"
)

// NewReleaseCmd returns the release command.
func NewReleaseCmd() *cobra.Command {
	var sinceFlag string
	var discoverFlag bool

	cmd := &cobra.Command{
		Use:   "release <tag>",
		Short: "Release velocity and quality metrics",
		Long: `Compute per-issue lead time, cycle time, release lag, and quality metrics for a release.

Use --discover to show the discovery diagnostic view: which issues and PRs each
linking strategy discovered for the release.`,
		Example: `  # Release metrics with auto-detected previous tag
  gh velocity quality release v2.65.0

  # Explicit previous tag
  gh velocity quality release v2.65.0 --since v2.64.0

  # Discover diagnostic: what did each strategy find?
  gh velocity quality release v2.65.0 --discover

  # Remote repo, JSON output
  gh velocity quality release v2.65.0 -R cli/cli -f json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tag := args[0]
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

			// Choose git data source: local git or API fallback
			var source gitdata.Source
			var preWarnings []string
			if deps.HasLocalRepo {
				wd, wdErr := os.Getwd()
				if wdErr != nil {
					return fmt.Errorf("get working directory: %w", wdErr)
				}
				if gitdata.IsShallowClone(wd) {
					w := "shallow clone detected; commit history is incomplete. Use 'actions/checkout' with fetch-depth: 0 for accurate metrics."
					deps.WarnUnlessJSON("%s", w)
					preWarnings = append(preWarnings, w)
				}
				source = gitdata.NewLocalSource(wd)
			} else {
				w := "Using API for git operations (no local checkout)"
				deps.WarnUnlessJSON("%s", w)
				preWarnings = append(preWarnings, w)
				source = gitdata.NewAPISource(client)
			}

			// Gather data: tags, commits, release info, issues
			input, scopeResult, warnings, err := gatherReleaseData(ctx, source, client, deps, tag, sinceFlag)
			warnings = append(preWarnings, warnings...)
			if err != nil {
				return err
			}

			// --discover: output scope diagnostic view and return
			if discoverFlag {
				for _, warning := range warnings {
					deps.WarnUnlessJSON("%s", warning)
				}
				w := cmd.OutOrStdout()
				switch deps.Format {
				case format.JSON:
					return format.WriteScopeJSON(w, deps.Owner+"/"+deps.Repo, scopeResult)
				case format.Markdown:
					return format.WriteScopeMarkdown(w, scopeResult)
				default:
					return format.WriteScopePretty(w, deps.IsTTY, deps.TermWidth, scopeResult)
				}
			}

			// Build classifier from config categories.
			classifier, classErr := classify.NewClassifier(deps.Config.Quality.Categories)
			if classErr != nil {
				return fmt.Errorf("invalid classification config: %w", classErr)
			}
			input.Classifier = classifier
			input.HotfixWindowHours = deps.Config.Quality.HotfixWindowHours
			input.CycleTimeStrategy = buildCycleTimeStrategy(ctx, deps, client)

			p := &release.Pipeline{
				Owner:    deps.Owner,
				Repo:     deps.Repo,
				Input:    input,
				Warnings: warnings,
			}

			if err := p.ProcessData(); err != nil {
				return err
			}

			w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
				Command: "release",
				Context: tag,
				Target:  posting.DiscussionTarget,
			})
			rc := deps.RenderCtx(w)
			if err := p.Render(rc); err != nil {
				return err
			}
			return postFn()
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Override previous tag for commit range (tag name)")
	cmd.Flags().BoolVar(&discoverFlag, "discover", false, "Show discovery diagnostic: what issues/PRs each strategy discovered")
	return cmd
}

// gatherReleaseData fetches all data needed for release metrics computation.
func gatherReleaseData(ctx context.Context, source gitdata.Source, client *gh.Client, deps *Deps, tag, sinceFlag string) (metrics.ReleaseInput, *model.ScopeResult, []string, error) {
	var warnings []string

	tags, err := source.Tags(ctx)
	if err != nil {
		return metrics.ReleaseInput{}, nil, nil, fmt.Errorf("list tags: %w", err)
	}
	previousTag := findPreviousTag(tags, tag, sinceFlag)

	// Get commits
	var commits []model.Commit
	if previousTag != "" {
		commits, err = source.CommitsBetween(ctx, previousTag, tag)
	} else {
		commits, err = source.AllCommits(ctx, tag)
		if err == nil && len(commits) > 500 {
			warnings = append(warnings, fmt.Sprintf("Large history: %d commits from initial commit. Use --since <tag> to limit scope.", len(commits)))
		}
	}
	if err != nil {
		return metrics.ReleaseInput{}, nil, nil, fmt.Errorf("get commits: %w", err)
	}

	// Get release
	release, err := client.GetRelease(ctx, tag)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("no GitHub release for %s, using current time for release date", tag))
		release = &model.Release{TagName: tag, CreatedAt: deps.Now()}
	}

	// Fetch previous release for hotfix/cadence detection
	var prevRelease *model.Release
	if previousTag != "" {
		if pr, err := client.GetRelease(ctx, previousTag); err == nil {
			prevRelease = pr
		}
	}

	// Determine tag dates for strategies.
	// Use release dates when available, fall back to git tag commit dates.
	tagDate := release.CreatedAt
	var prevTagDate time.Time
	if prevRelease != nil {
		prevTagDate = prevRelease.CreatedAt
	} else if previousTag != "" {
		if d, err := client.GetTagDate(ctx, previousTag); err == nil {
			prevTagDate = d
		}
	}

	// Run linking strategies to discover issues and PRs
	runner := strategy.NewRunner(
		strategy.NewPRLink(),
		strategy.NewCommitRef(deps.Config.CommitRef.Patterns),
		strategy.NewChangelog(),
	)

	scopeResult, stratWarnings, err := runner.Run(ctx, strategy.DiscoverInput{
		Owner:             deps.Owner,
		Repo:              deps.Repo,
		Tag:               tag,
		PreviousTag:       previousTag,
		TagDate:           tagDate,
		PrevTagDate:       prevTagDate,
		Commits:           commits,
		Release:           release,
		Client:            client,
		Scope:             deps.Scope,
		CommitRefPatterns: deps.Config.CommitRef.Patterns,
	})
	warnings = append(warnings, stratWarnings...)
	if err != nil {
		return metrics.ReleaseInput{}, nil, warnings, err
	}

	// Convert strategy results to metrics input format.
	// Collect issue numbers that need full data fetching.
	issueCommits := make(map[int][]model.Commit)
	knownIssues := make(map[int]*model.Issue)
	linkedPRs := make(map[int]*model.PR)

	for _, item := range scopeResult.Merged {
		if item.Issue == nil {
			continue
		}
		num := item.Issue.Number
		issueCommits[num] = append(issueCommits[num], item.Commits...)

		// pr-link provides full issue data; commit-ref/changelog only have Number.
		if item.Issue.Title != "" {
			knownIssues[num] = item.Issue
		}

		// Track linked PRs for cycle-time PR strategy.
		if item.PR != nil {
			linkedPRs[num] = item.PR
		}
	}

	// Fetch full issue data for issues not already populated by pr-link.
	var toFetch []int
	for num := range issueCommits {
		if _, ok := knownIssues[num]; !ok {
			toFetch = append(toFetch, num)
		}
	}

	issues := make(map[int]*model.Issue)
	fetchErrors := make(map[int]error)

	// Copy known issues
	maps.Copy(issues, knownIssues)

	// Fetch remaining issues
	if len(toFetch) > 0 {
		fetched, errs := client.FetchIssues(ctx, toFetch)
		maps.Copy(issues, fetched)
		maps.Copy(fetchErrors, errs)
	}

	input := metrics.ReleaseInput{
		Tag:          tag,
		PreviousTag:  previousTag,
		Release:      *release,
		PrevRelease:  prevRelease,
		IssueCommits: issueCommits,
		Issues:       issues,
		LinkedPRs:    linkedPRs,
		FetchErrors:  fetchErrors,
	}
	return input, scopeResult, warnings, nil
}

func findPreviousTag(tags []string, currentTag, sinceFlag string) string {
	if sinceFlag != "" {
		return sinceFlag
	}
	for i, t := range tags {
		if t == currentTag && i+1 < len(tags) {
			return tags[i+1]
		}
	}
	return ""
}
