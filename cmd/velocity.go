package cmd

import (
	"fmt"
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/config"
	"github.com/bitsbyme/gh-velocity/internal/dateutil"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/pipeline/velocity"
	"github.com/bitsbyme/gh-velocity/internal/posting"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NewVelocityCmd returns the velocity command.
func NewVelocityCmd() *cobra.Command {
	var (
		sinceFlag, untilFlag string
		iterationsFlag       int
		currentFlag          bool
		historyFlag          bool
		verboseFlag          bool
	)

	cmd := &cobra.Command{
		Use:   "velocity",
		Short: "Measure effort completed per iteration (sprint velocity)",
		Long: `Velocity measures effort completed per iteration and completion rate.

Three effort strategies:
  count     — every item = 1 (default)
  attribute — map labels/types to effort values via matchers
  numeric   — read effort from a project board Number field

Two iteration strategies:
  project-field — read boundaries from a ProjectV2 Iteration field
  fixed         — calendar math from length + anchor date

Configure in .gh-velocity.yml under the velocity: section.
Run 'gh velocity config preflight' to get suggested configuration.`,
		Example: `  # Default: count effort, last 6 iterations
  gh velocity flow velocity

  # Current iteration only
  gh velocity flow velocity --current

  # Last 3 iterations, JSON output
  gh velocity flow velocity --history --iterations 3 --format json

  # With date filter
  gh velocity flow velocity --since 2026-01-01 --until 2026-03-01

  # Show not-assessed item numbers
  gh velocity flow velocity --verbose`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			deps := DepsFromContext(ctx)
			if deps == nil {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "internal error: missing dependencies",
				}
			}

			now := deps.Now()
			cfg := deps.Config.Velocity

			// Validate that iteration strategy is configured.
			if cfg.Iteration.Strategy == "" {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "velocity.iteration.strategy is not configured; run 'gh velocity config preflight' or set it in .gh-velocity.yml",
				}
			}

			client, err := deps.NewClient()
			if err != nil {
				return err
			}

			iterCount := cfg.Iteration.Count
			if iterationsFlag > 0 {
				iterCount = iterationsFlag
			}

			p := &velocity.Pipeline{
				Client:         client,
				Owner:          deps.Owner,
				Repo:           deps.Repo,
				Config:         cfg,
				ProjectConfig:  deps.Config.Project,
				Scope:          deps.Scope,
				ExcludeUsers:   deps.ExcludeUsers,
				Now:            now,
				ShowCurrent:    currentFlag,
				ShowHistory:    historyFlag,
				IterationCount: iterCount,
				Verbose:        verboseFlag,
			}

			if sinceFlag != "" {
				t, err := dateutil.Parse(sinceFlag, now)
				if err != nil {
					return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
				}
				p.Since = &t
			}
			if untilFlag != "" {
				t, err := dateutil.Parse(untilFlag, now)
				if err != nil {
					return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
				}
				p.Until = &t
			}

			if deps.Debug {
				log.Debug("velocity config: unit=%s effort=%s iteration=%s count=%d",
					cfg.Unit, cfg.Effort.Strategy, cfg.Iteration.Strategy, iterCount)
			}

			if err := p.GatherData(ctx); err != nil {
				return err
			}
			if err := p.ProcessData(); err != nil {
				return err
			}
			p.Result.Provenance = buildVelocityProvenance(cmd, deps, cfg)

			w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
				Command: "velocity",
				Context: dateutil.FormatContext(sinceFlag, untilFlag),
				Target:  posting.DiscussionTarget,
			})
			rc := deps.RenderCtx(w)
			if err := p.Render(rc); err != nil {
				return err
			}
			return postFn()
		},
	}

	cmd.Flags().StringVar(&sinceFlag, "since", "", "Show iterations overlapping this start date")
	cmd.Flags().StringVar(&untilFlag, "until", "", "Show iterations overlapping this end date")
	cmd.Flags().IntVar(&iterationsFlag, "iterations", 0, "Number of past iterations to show (overrides config)")
	cmd.Flags().BoolVar(&currentFlag, "current", false, "Show only the current iteration")
	cmd.Flags().BoolVar(&historyFlag, "history", false, "Show only past iterations")
	cmd.Flags().BoolVar(&verboseFlag, "verbose", false, "Include not-assessed item numbers")

	return cmd
}

// buildVelocityProvenance captures the command and key config for reproducibility.
func buildVelocityProvenance(cmd *cobra.Command, deps *Deps, cfg config.VelocityConfig) model.Provenance {
	// Reconstruct command line from explicitly-set flags.
	seen := map[string]bool{}
	var parts []string
	parts = append(parts, "gh velocity flow velocity")
	// --repo is context, not a filter — capture it in config, not the command.
	seen["repo"] = true
	addFlag := func(f *pflag.Flag) {
		if seen[f.Name] {
			return
		}
		seen[f.Name] = true
		switch f.Value.Type() {
		case "bool":
			parts = append(parts, "--"+f.Name)
		default:
			parts = append(parts, fmt.Sprintf("--%s %s", f.Name, f.Value.String()))
		}
	}
	cmd.Flags().Visit(addFlag)
	cmd.InheritedFlags().Visit(addFlag)

	// Capture config values that affect interpretation.
	cfgMap := map[string]string{
		"run_in":             deps.Owner + "/" + deps.Repo,
		"unit":               cfg.Unit,
		"effort_strategy":    cfg.Effort.Strategy,
		"iteration_strategy": cfg.Iteration.Strategy,
	}
	if cfg.Iteration.Strategy == "fixed" {
		cfgMap["iteration_length"] = cfg.Iteration.Fixed.Length
	}
	if cfg.Iteration.Strategy == "project-field" {
		cfgMap["iteration_field"] = cfg.Iteration.ProjectField
	}
	if cfg.Effort.Strategy == "numeric" {
		cfgMap["effort_field"] = cfg.Effort.Numeric.ProjectField
	}
	if deps.Config.Project.URL != "" {
		cfgMap["project_url"] = deps.Config.Project.URL
	}

	return model.Provenance{
		Command: strings.Join(parts, " "),
		Config:  cfgMap,
	}
}
