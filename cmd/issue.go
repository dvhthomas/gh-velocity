package cmd

import (
	"strconv"

	"github.com/dvhthomas/gh-velocity/internal/classify"
	"github.com/dvhthomas/gh-velocity/internal/model"
	issuepipe "github.com/dvhthomas/gh-velocity/internal/pipeline/issue"
	"github.com/dvhthomas/gh-velocity/internal/posting"
	"github.com/spf13/cobra"
)

// NewIssueCmd returns the top-level issue detail command.
func NewIssueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <number>",
		Short: "Composite detail view for a single issue",
		Long: `Show everything about a single issue: facts (timestamps, category),
metrics (lead time, cycle time), and linked PRs with their cycle times.

This is the recommended command for per-item GitHub Actions automation.
Use --post to add a rich comment to the issue when it closes.`,
		Example: `  # View issue detail
  gh velocity issue 42

  # Post as a comment on the issue
  gh velocity issue 42 --post

  # JSON output
  gh velocity issue 42 -r json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIssueDetail(cmd, args[0])
		},
	}

	return cmd
}

func runIssueDetail(cmd *cobra.Command, arg string) error {
	issueNumber, err := parseIssueArg(arg)
	if err != nil {
		return err
	}

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

	// Build cycle time strategy (same as cycle-time command).
	strategy := buildCycleTimeStrategy(ctx, deps, client)

	// Build classifier from config categories.
	var classifier *classify.Classifier
	if len(deps.Config.Quality.Categories) > 0 {
		c, cErr := classify.NewClassifier(deps.Config.Quality.Categories)
		if cErr != nil {
			deps.Warn("could not build classifier: %v", cErr)
		} else {
			classifier = c
		}
	}

	p := &issuepipe.Pipeline{
		Client:            client,
		Owner:             deps.Owner,
		Repo:              deps.Repo,
		IssueNumber:       issueNumber,
		Strategy:          strategy,
		Classifier:        classifier,
		HasLifecycleMatch: len(deps.Config.Lifecycle.InProgress.Match) > 0,
	}

	if err := p.GatherData(ctx); err != nil {
		return err
	}
	if err := p.ProcessData(); err != nil {
		return err
	}

	// Surface pipeline warnings.
	for _, w := range p.Warnings {
		deps.Warn("%s", w)
	}

	return renderPipeline(cmd, deps, p, client, posting.PostOptions{
		Command: "issue",
		Context: strconv.Itoa(issueNumber),
		Target:  posting.IssueBody,
		Number:  issueNumber,
		Repo:    deps.Owner + "/" + deps.Repo,
	})
}
