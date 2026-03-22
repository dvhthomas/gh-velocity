package cmd

import (
	"strconv"

	"github.com/dvhthomas/gh-velocity/internal/model"
	prpipe "github.com/dvhthomas/gh-velocity/internal/pipeline/pr"
	"github.com/dvhthomas/gh-velocity/internal/posting"
	"github.com/spf13/cobra"
)

// NewPRCmd returns the top-level PR detail command.
func NewPRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <number>",
		Short: "Composite detail view for a single pull request",
		Long: `Show everything about a single PR: facts (author, timestamps),
metrics (cycle time, time to first review, review rounds), and closed issues.

This is the recommended command for per-PR GitHub Actions automation.
Use --post to add a rich comment to the PR when it merges.`,
		Example: `  # View PR detail
  gh velocity pr 125

  # Post as a comment on the PR
  gh velocity pr 125 --post

  # JSON output
  gh velocity pr 125 -r json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPRDetail(cmd, args[0])
		},
	}

	return cmd
}

func runPRDetail(cmd *cobra.Command, arg string) error {
	prNumber, err := parseIssueArg(arg)
	if err != nil {
		return err
	}

	deps := DepsFromContext(cmd.Context())
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

	p := &prpipe.Pipeline{
		Client:   client,
		Owner:    deps.Owner,
		Repo:     deps.Repo,
		PRNumber: prNumber,
	}

	return renderPipeline(cmd, deps, p, client, posting.PostOptions{
		Command: "pr",
		Context: strconv.Itoa(prNumber),
		Target:  posting.IssueBody,
		Number:  prNumber,
		Repo:    deps.Owner + "/" + deps.Repo,
	})
}
