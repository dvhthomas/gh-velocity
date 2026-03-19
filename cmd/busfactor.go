package cmd

import (
	"os"

	"github.com/dvhthomas/gh-velocity/internal/dateutil"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/pipeline/busfactor"
	"github.com/spf13/cobra"
)

const (
	busFactorDepth      = 2
	busFactorMinCommits = 5
)

// NewBusFactorCmd returns the bus-factor command.
func NewBusFactorCmd() *cobra.Command {
	var sinceFlag string

	cmd := &cobra.Command{
		Use:   "bus-factor",
		Short: "Knowledge risk per directory from git history",
		Long: `Analyzes local git history to identify directories where knowledge is
concentrated in one or two people. Helps spot areas that would stall
if a key contributor became unavailable.

Risk levels:
  HIGH   — 1 contributor
  MEDIUM — 2 contributors, primary >70% of commits
  LOW    — 3+ contributors with distributed commits`,
		Example: `  # Last 90 days (default)
  gh velocity quality bus-factor

  # Last 180 days
  gh velocity quality bus-factor --since 180d

  # JSON for CI/scripts
  gh velocity quality bus-factor --results json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBusFactor(cmd, sinceFlag)
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "90d", "Lookback period (YYYY-MM-DD, RFC3339, or Nd relative)")
	return cmd
}

func runBusFactor(cmd *cobra.Command, sinceStr string) error {
	ctx := cmd.Context()
	deps := DepsFromContext(ctx)
	if deps == nil {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "internal error: missing dependencies",
		}
	}

	if !deps.HasLocalRepo {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "bus-factor requires a local git checkout. Run from within the repository or use a GitHub Action with actions/checkout.",
		}
	}

	now := deps.Now()
	since, err := dateutil.Parse(sinceStr, now)
	if err != nil {
		return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
	}

	wd, err := os.Getwd()
	if err != nil {
		return &model.AppError{Code: model.ErrNotGitRepo, Message: "could not determine working directory: " + err.Error()}
	}

	if deps.Debug {
		log.Debug("bus-factor: since=%s depth=%d min-commits=%d", since.Format("2006-01-02"), busFactorDepth, busFactorMinCommits)
	}

	p := &busfactor.Pipeline{
		Repository: deps.Owner + "/" + deps.Repo,
		WorkDir:    wd,
		Since:      since,
		Depth:      busFactorDepth,
		MinCommits: busFactorMinCommits,
	}

	if err := p.GatherData(ctx); err != nil {
		return err
	}
	if err := p.ProcessData(); err != nil {
		return err
	}
	return renderPipelineSimple(cmd, deps, p)
}
