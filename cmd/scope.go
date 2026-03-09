package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/gitdata"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/strategy"
	"github.com/spf13/cobra"
)

// NewScopeCmd returns the scope command.
func NewScopeCmd() *cobra.Command {
	var sinceFlag string

	cmd := &cobra.Command{
		Use:   "scope <tag>",
		Short: "Show what a release contains",
		Long:  "Validate which issues and PRs each linking strategy discovers for a release.",
		Args:  cobra.ExactArgs(1),
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

			client, err := gh.NewClient(deps.Owner, deps.Repo)
			if err != nil {
				return err
			}

			// Choose git data source
			var source gitdata.Source
			if deps.HasLocalRepo {
				wd, wdErr := os.Getwd()
				if wdErr != nil {
					return fmt.Errorf("get working directory: %w", wdErr)
				}
				if gitdata.IsShallowClone(wd) {
					fmt.Fprintf(os.Stderr, "warning: shallow clone detected; commit history is incomplete. Use 'actions/checkout' with fetch-depth: 0 for accurate metrics.\n")
				}
				source = gitdata.NewLocalSource(wd)
			} else {
				fmt.Fprintf(os.Stderr, "warning: Using API for git operations (no local checkout)\n")
				source = gitdata.NewAPISource(client)
			}

			// Get tags and commits
			tags, err := source.Tags(ctx)
			if err != nil {
				return fmt.Errorf("list tags: %w", err)
			}
			previousTag := findPreviousTag(tags, tag, sinceFlag)

			var commits []model.Commit
			if previousTag != "" {
				commits, err = source.CommitsBetween(ctx, previousTag, tag)
			} else {
				commits, err = source.AllCommits(ctx, tag)
			}
			if err != nil {
				return fmt.Errorf("get commits: %w", err)
			}

			// Get release info
			release, err := client.GetRelease(ctx, tag)
			if err != nil {
				now := time.Now()
				release = &model.Release{TagName: tag, CreatedAt: now}
			}

			// Get previous tag date — try release first, fall back to git tag date
			var prevTagDate time.Time
			if previousTag != "" {
				if pr, err := client.GetRelease(ctx, previousTag); err == nil {
					prevTagDate = pr.CreatedAt
				} else if d, err := client.GetTagDate(ctx, previousTag); err == nil {
					prevTagDate = d
				}
			}

			// Run strategies
			runner := strategy.NewRunner(
				strategy.NewPRLink(),
				strategy.NewCommitRef(deps.Config.CommitRef.Patterns),
				strategy.NewChangelog(),
			)

			scopeResult, warnings, err := runner.Run(ctx, strategy.DiscoverInput{
				Owner:             deps.Owner,
				Repo:              deps.Repo,
				Tag:               tag,
				PreviousTag:       previousTag,
				TagDate:           release.CreatedAt,
				PrevTagDate:       prevTagDate,
				Commits:           commits,
				Release:           release,
				Client:            client,
				CommitRefPatterns: deps.Config.CommitRef.Patterns,
			})
			if err != nil {
				return err
			}

			// Print warnings
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}

			// Output
			w := cmd.OutOrStdout()
			switch deps.Format {
			case format.JSON:
				return format.WriteScopeJSON(w, deps.Owner+"/"+deps.Repo, scopeResult)
			case format.Markdown:
				return format.WriteScopeMarkdown(w, scopeResult)
			default:
				return format.WriteScopePretty(w, scopeResult)
			}
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Previous tag for commit range")
	return cmd
}
