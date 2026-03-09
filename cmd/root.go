// Package cmd implements the Cobra commands for gh-velocity.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/bitsbyme/gh-velocity/internal/config"
	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/gitdata"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/spf13/cobra"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const configKey contextKey = "config"

// Deps holds shared dependencies injected into subcommands.
type Deps struct {
	Config       *config.Config
	Format       format.Format
	Post         bool
	Owner        string
	Repo         string
	HasLocalRepo bool // true when a local git checkout is available
}

// DepsFromContext extracts Deps from the command context.
func DepsFromContext(ctx context.Context) *Deps {
	if d, ok := ctx.Value(configKey).(*Deps); ok {
		return d
	}
	return nil
}

// Execute runs the root command and returns an exit code.
func Execute(version, buildTime string) int {
	root := NewRootCmd(version, buildTime)
	if err := root.Execute(); err != nil {
		return 1
	}
	return 0
}

// NewRootCmd creates and returns the root command with all subcommands wired.
func NewRootCmd(version, buildTime string) *cobra.Command {
	var (
		formatFlag string
		repoFlag   string
		postFlag   bool
	)

	root := &cobra.Command{
		Use:   "gh-velocity",
		Short: "GitHub velocity and quality metrics",
		Long:  "Compute velocity and quality metrics from GitHub data and post them where the work happens.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip config loading for version and config subcommands
			if cmd.Name() == "version" || cmd.Parent().Name() == "config" {
				return nil
			}

			// Reject --post until posting is implemented
			if postFlag {
				return fmt.Errorf("--post is not yet implemented; default output is read-only")
			}

			// Validate format
			f, err := format.ParseFormat(formatFlag)
			if err != nil {
				return err
			}

			// Resolve repo
			owner, repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}

			// Detect local git availability
			wd, _ := os.Getwd()
			hasLocal := gitdata.IsLocalGitAvailable(wd)

			// Load config: skip file discovery when no local repo
			var cfg *config.Config
			if hasLocal {
				cfg, err = config.Load(config.DefaultConfigFile)
				if err != nil {
					return err
				}
			} else {
				cfg, err = config.Load(config.DefaultConfigFile)
				if err != nil {
					// Config file not found is fine when running without
					// a local repo — use defaults.
					cfg = config.Defaults()
				}
			}

			deps := &Deps{
				Config:       cfg,
				Format:       f,
				Post:         postFlag,
				Owner:        owner,
				Repo:         repo,
				HasLocalRepo: hasLocal,
			}

			cmd.SetContext(context.WithValue(cmd.Context(), configKey, deps))
			return nil
		},
	}

	root.PersistentFlags().StringVarP(&formatFlag, "format", "f", "pretty", "Output format: json, pretty, markdown")
	root.PersistentFlags().StringVarP(&repoFlag, "repo", "R", "", "Repository in owner/name format")
	root.PersistentFlags().BoolVar(&postFlag, "post", false, "Post output to GitHub")

	root.AddCommand(NewVersionCmd(version, buildTime))
	root.AddCommand(NewConfigCmd())
	root.AddCommand(NewReleaseCmd())
	root.AddCommand(NewLeadTimeCmd())
	root.AddCommand(NewCycleTimeCmd())

	return root
}

// resolveRepo determines the target repository from --repo flag,
// GH_REPO env, or git remote (via go-gh).
func resolveRepo(flag string) (string, string, error) {
	// --repo flag takes priority.
	if flag != "" {
		r, err := repository.Parse(flag)
		if err != nil {
			return "", "", fmt.Errorf("invalid --repo %q: must be owner/name", flag)
		}
		return r.Owner, r.Name, nil
	}

	// GH_REPO environment variable.
	if env := os.Getenv("GH_REPO"); env != "" {
		r, err := repository.Parse(env)
		if err != nil {
			return "", "", fmt.Errorf("invalid GH_REPO %q: must be owner/name", env)
		}
		return r.Owner, r.Name, nil
	}

	// Fall back to git remote detection.
	r, err := repository.Current()
	if err != nil {
		return "", "", fmt.Errorf("not a git repository. Use --repo owner/name")
	}
	return r.Owner, r.Name, nil
}
