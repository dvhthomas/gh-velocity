package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/gitdata"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/linking"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

// NewReleaseCmd returns the release command.
func NewReleaseCmd() *cobra.Command {
	var sinceFlag string

	cmd := &cobra.Command{
		Use:   "release <tag>",
		Short: "Release velocity and quality metrics",
		Long:  "Compute per-issue lead time, cycle time, release lag, and quality metrics for a release.",
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

			// Choose git data source: local git or API fallback
			var source gitdata.Source
			if deps.HasLocalRepo {
				wd, wdErr := os.Getwd()
				if wdErr != nil {
					return fmt.Errorf("get working directory: %w", wdErr)
				}
				source = gitdata.NewLocalSource(wd)
			} else {
				fmt.Fprintf(os.Stderr, "warning: Using API for git operations (no local checkout)\n")
				source = gitdata.NewAPISource(client)
			}

			// Gather data: tags, commits, release info, issues
			input, warnings, err := gatherReleaseData(ctx, source, client, tag, sinceFlag)
			if err != nil {
				return err
			}
			input.BugLabels = deps.Config.Quality.BugLabels
			input.FeatureLabels = deps.Config.Quality.FeatureLabels
			input.HotfixWindowHours = deps.Config.Quality.HotfixWindowHours

			// Compute metrics
			rm, metricWarnings, err := metrics.BuildReleaseMetrics(input)
			if err != nil {
				return err
			}
			warnings = append(warnings, metricWarnings...)

			// Output
			w := cmd.OutOrStdout()
			switch deps.Format {
			case format.JSON:
				return format.WriteReleaseJSON(w, deps.Owner+"/"+deps.Repo, rm, warnings)
			case format.Markdown:
				return format.WriteReleaseMarkdown(w, rm, warnings)
			default:
				return format.WriteReleasePretty(w, rm, warnings)
			}
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Override previous tag for commit range (tag name)")
	return cmd
}

// gatherReleaseData fetches all data needed for release metrics computation.
func gatherReleaseData(ctx context.Context, source gitdata.Source, client *gh.Client, tag, sinceFlag string) (metrics.ReleaseInput, []string, error) {
	var warnings []string

	tags, err := source.Tags(ctx)
	if err != nil {
		return metrics.ReleaseInput{}, nil, fmt.Errorf("list tags: %w", err)
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
		return metrics.ReleaseInput{}, nil, fmt.Errorf("get commits: %w", err)
	}

	// Get release
	release, err := client.GetRelease(ctx, tag)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("no GitHub release for %s, using current time for release date", tag))
		now := time.Now()
		release = &model.Release{TagName: tag, CreatedAt: now}
	}

	// Link commits to issues and fetch issue data concurrently
	issueCommits := linking.LinkCommitsToIssues(commits)
	issueNumbers := make([]int, 0, len(issueCommits))
	for num := range issueCommits {
		issueNumbers = append(issueNumbers, num)
	}
	issues, fetchErrors := client.FetchIssues(ctx, issueNumbers)

	// Fetch previous release for hotfix/cadence detection
	var prevRelease *model.Release
	if previousTag != "" {
		if pr, err := client.GetRelease(ctx, previousTag); err == nil {
			prevRelease = pr
		}
	}

	input := metrics.ReleaseInput{
		Tag:          tag,
		PreviousTag:  previousTag,
		Release:      *release,
		PrevRelease:  prevRelease,
		IssueCommits: issueCommits,
		Issues:       issues,
		FetchErrors:  fetchErrors,
	}
	return input, warnings, nil
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
