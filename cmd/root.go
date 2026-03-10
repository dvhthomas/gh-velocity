// Package cmd implements the Cobra commands for gh-velocity.
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/config"
	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/gitdata"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/cli/go-gh/v2/pkg/term"
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
	IsTTY        bool // true when stdout is a terminal
	TermWidth    int  // terminal width in columns (0 = unknown)
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
		return handleError(root, err)
	}
	return 0
}

// handleError processes the error from command execution, emitting JSON
// error output to stderr when --format json is set, and returning the
// appropriate exit code.
func handleError(root *cobra.Command, err error) int {
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// Check if JSON format was requested
	formatFlag, _ := root.PersistentFlags().GetString("format")
	if formatFlag == "json" {
		envelope := model.ErrorEnvelope{Error: appErr}
		data, jsonErr := json.Marshal(envelope)
		if jsonErr == nil {
			fmt.Fprintln(os.Stderr, string(data))
		} else {
			fmt.Fprintln(os.Stderr, appErr.Error())
		}
	} else {
		fmt.Fprintln(os.Stderr, appErr.Error())
	}

	return appErr.ExitCode()
}

// NewRootCmd creates and returns the root command with all subcommands wired.
func NewRootCmd(version, buildTime string) *cobra.Command {
	var (
		formatFlag string
		repoFlag   string
		postFlag   bool
	)

	root := &cobra.Command{
		Use:           "gh-velocity",
		Short:         "GitHub velocity and quality metrics",
		Long:          "Compute velocity and quality metrics from GitHub data and post them where the work happens.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip config loading for version and config subcommands
			if cmd.Name() == "version" || cmd.Parent().Name() == "config" {
				return nil
			}

			// Reject --post until posting is implemented
			if postFlag {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "--post is not yet implemented; default output is read-only",
				}
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

			// Detect local git availability — only use local git when
			// the working directory is a git repo whose remote matches
			// the resolved owner/repo target.
			wd, _ := os.Getwd()
			hasLocal := gitdata.IsLocalGitAvailable(wd) && localRepoMatches(wd, owner, repo)

			// Load config: require file when local repo exists, fall back to defaults otherwise.
			var cfg *config.Config
			cfg, err = config.Load(config.DefaultConfigFile)
			if err != nil {
				if hasLocal {
					return err
				}
				// Config file not found is fine when running without
				// a local repo — use defaults.
				cfg = config.Defaults()
			}

			// Detect terminal capabilities for pretty output.
			t := term.FromEnv()
			isTTY := t.IsTerminalOutput()
			termWidth := 80
			if w, _, err := t.Size(); err == nil && w > 0 {
				termWidth = w
			}

			deps := &Deps{
				Config:       cfg,
				Format:       f,
				Post:         postFlag,
				Owner:        owner,
				Repo:         repo,
				HasLocalRepo: hasLocal,
				IsTTY:        isTTY,
				TermWidth:    termWidth,
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
	root.AddCommand(NewScopeCmd())

	return root
}

// parseIssueArg parses and validates an issue number from a command argument.
func parseIssueArg(arg string) (int, error) {
	n, err := strconv.Atoi(arg)
	if err != nil {
		return 0, &model.AppError{Code: model.ErrConfigInvalid, Message: fmt.Sprintf("invalid issue number %q: must be a positive integer", arg)}
	}
	if n <= 0 {
		return 0, &model.AppError{Code: model.ErrConfigInvalid, Message: fmt.Sprintf("invalid issue number %d: must be a positive integer", n)}
	}
	return n, nil
}

// localRepoMatches returns true when the git remote in dir matches
// the target owner/repo. This prevents using local git operations
// against the wrong repository when -R points elsewhere.
func localRepoMatches(dir, owner, repo string) bool {
	r, err := repository.Current()
	if err != nil {
		return false
	}
	return strings.EqualFold(r.Owner, owner) && strings.EqualFold(r.Name, repo)
}

// resolveRepo determines the target repository from --repo flag,
// GH_REPO env, or git remote (via go-gh).
func resolveRepo(flag string) (string, string, error) {
	// --repo flag takes priority.
	if flag != "" {
		r, err := repository.Parse(flag)
		if err != nil {
			return "", "", fmt.Errorf("invalid --repo %q: %w", flag, err)
		}
		return r.Owner, r.Name, nil
	}

	// GH_REPO environment variable.
	if env := os.Getenv("GH_REPO"); env != "" {
		r, err := repository.Parse(env)
		if err != nil {
			return "", "", fmt.Errorf("invalid GH_REPO %q: %w", env, err)
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
