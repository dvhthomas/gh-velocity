// Package cmd implements the Cobra commands for gh-velocity.
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/config"
	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/gitdata"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/scope"
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
	NewPost      bool // --new-post: force a new post (skip idempotent update)
	DryRun       bool // true unless GH_VELOCITY_POST_LIVE=true — protects against accidental mutations
	Owner        string
	Repo         string
	Scope        string           // merged config + flag scope (GitHub search query fragment)
	ExcludeUsers string           // "-author:bot1 -author:bot2" exclusion qualifiers from config
	HasLocalRepo bool             // true when a local git checkout is available
	IsTTY        bool             // true when stdout is a terminal
	TermWidth    int              // terminal width in columns (0 = unknown)
	Debug        bool             // true when --debug is set
	NoCache      bool             // true when --no-cache is set (disables disk cache)
	Now          func() time.Time // returns current time; override via GH_VELOCITY_NOW for testing
}

// nowFunc returns a function that provides the current time.
// If GH_VELOCITY_NOW is set, the returned function always returns that fixed time.
func nowFunc() func() time.Time {
	if env := os.Getenv("GH_VELOCITY_NOW"); env != "" {
		if t, err := time.Parse(time.RFC3339, env); err == nil {
			return func() time.Time { return t }
		}
		// Also accept date-only format.
		if t, err := time.Parse(time.DateOnly, env); err == nil {
			return func() time.Time { return t.UTC() }
		}
	}
	return func() time.Time { return time.Now().UTC() }
}

// NewClient creates a GitHub API client with the configured API throttle applied.
func (d *Deps) NewClient() (*gh.Client, error) {
	var delay time.Duration
	if d.Config != nil {
		delay = d.Config.APIThrottleDuration()
	}
	return gh.NewClient(d.Owner, d.Repo, delay, gh.ClientOptions{NoCache: d.NoCache})
}

// WarnUnlessJSON emits a warning to stderr unless JSON format is selected.
// In JSON mode, warnings are included in the JSON payload — stderr warnings
// would be noise for agentic consumers.
func (d *Deps) WarnUnlessJSON(format string, args ...any) {
	if d.Format != "json" {
		log.Warn(format, args...)
	}
}

// RenderCtx builds a format.RenderContext from Deps and a writer.
func (d *Deps) RenderCtx(w io.Writer) format.RenderContext {
	return format.RenderContext{
		Writer: w,
		Format: d.Format,
		IsTTY:  d.IsTTY,
		Width:  d.TermWidth,
		Owner:  d.Owner,
		Repo:   d.Repo,
	}
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
// appropriate exit code. All errors are wrapped as structured JSON for
// agentic consumers when JSON format is requested.
func handleError(root *cobra.Command, err error) int {
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		// Wrap non-AppError into a generic AppError for consistent output.
		appErr = &model.AppError{
			Code:    "INTERNAL",
			Message: err.Error(),
		}
	}

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
		formatFlag  string
		repoFlag    string
		configFlag  string
		scopeFlag   string
		postFlag    bool
		newPostFlag bool
		debugFlag   bool
		noCacheFlag bool
	)

	root := &cobra.Command{
		Use:           "gh-velocity",
		Short:         "GitHub velocity and quality metrics",
		Long:          "Compute velocity and quality metrics from GitHub data and post them where the work happens.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip Deps setup for commands that don't need it.
			// Group parents (flow, status) print help only — no RunE.
			switch {
			case cmd.Name() == "version":
				return nil
			case cmd.Parent() != nil && cmd.Parent().Name() == "config":
				return nil
			case cmd.RunE == nil && cmd.Run == nil:
				return nil
			}

			// --new-post implies --post.
			if newPostFlag {
				postFlag = true
			}

			// --post coerces format to markdown unless user explicitly set -f.
			if postFlag && !cmd.Flags().Changed("format") {
				formatFlag = "markdown"
			}

			// Validate format
			f, err := format.ParseFormat(formatFlag)
			if err != nil {
				return err
			}

			// Suppress all non-error stderr output in JSON mode — warnings
			// and debug lines are noise for agentic consumers. Warnings
			// are embedded in the JSON payload instead.
			if f == format.JSON {
				log.SuppressStderr = true
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

			// Load config: use --config flag if set, otherwise default file.
			configPath := config.DefaultConfigFile
			if configFlag != "" {
				configPath = configFlag
			}
			// Config is required for all non-config commands.
			if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: fmt.Sprintf("no config found (%s)\n\n  To generate a starter config:  gh velocity config create\n  To auto-detect your setup:    gh velocity config preflight -R owner/repo --write", configPath),
				}
			}
			var cfg *config.Config
			cfg, err = config.Load(configPath)
			if err != nil {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: fmt.Sprintf("%v\n\n  To generate a starter config:  gh velocity config create\n  To auto-detect your setup:    gh velocity config preflight -R owner/repo --write", err),
				}
			}

			// Detect terminal capabilities for pretty output.
			t := term.FromEnv()
			isTTY := t.IsTerminalOutput()
			termWidth := 80
			if w, _, err := t.Size(); err == nil && w > 0 {
				termWidth = w
			}

			// Resolve scope: merge config scope + --scope flag.
			resolvedScope := scope.MergeScope(cfg.Scope.Query, scopeFlag)

			// Dry-run is the default for --post. Mutations only happen when
			// GH_VELOCITY_POST_LIVE=true is explicitly set. This prevents
			// tests, agents, and accidental runs from mutating GitHub state.
			dryRun := postFlag && os.Getenv("GH_VELOCITY_POST_LIVE") != "true"

			if debugFlag {
				repoSource := ""
				if isRepoAutoDetected(repoFlag) {
					repoSource = " (auto-detected from git remote)"
				}
				log.Debug("repo:         %s/%s%s", owner, repo, repoSource)
				log.Debug("local repo:   %v", hasLocal)
				log.Debug("config:       %s", configPath)
				log.Debug("format:       %s", formatFlag)
				log.Debug("scope:        %s", resolvedScope)
				log.Debug("strategy:     %s", cfg.CycleTime.Strategy)
				if cfg.Project.URL != "" {
					log.Debug("project.url:  %s", cfg.Project.URL)
				}
				if postFlag {
					mode := "dry-run"
					if !dryRun {
						mode = "live"
					}
					log.Debug("post:         %s", mode)
				}
			}

			deps := &Deps{
				Config:       cfg,
				Format:       f,
				Post:         postFlag,
				NewPost:      newPostFlag,
				DryRun:       dryRun,
				Owner:        owner,
				Repo:         repo,
				Scope:        resolvedScope,
				ExcludeUsers: scope.BuildExclusions(cfg.ExcludeUsers),
				HasLocalRepo: hasLocal,
				IsTTY:        isTTY,
				TermWidth:    termWidth,
				Debug:        debugFlag,
				NoCache:      noCacheFlag,
				Now:          nowFunc(),
			}

			cmd.SetContext(context.WithValue(cmd.Context(), configKey, deps))
			return nil
		},
	}

	root.PersistentFlags().StringVarP(&formatFlag, "format", "f", "pretty", "Output format: json, pretty, markdown")
	root.PersistentFlags().StringVarP(&repoFlag, "repo", "R", "", "Repository in owner/name format")
	root.PersistentFlags().StringVar(&configFlag, "config", "", "Path to config file (default: .gh-velocity.yml)")
	root.PersistentFlags().BoolVar(&postFlag, "post", false, "Post output to GitHub (dry-run by default; set GH_VELOCITY_POST_LIVE=true for live)")
	root.PersistentFlags().BoolVar(&newPostFlag, "new-post", false, "Force a new post (skip idempotent update; implies --post)")
	root.PersistentFlags().StringVar(&scopeFlag, "scope", "", "Additional GitHub search qualifier(s) ANDed with config scope")
	root.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Print diagnostic info to stderr")
	root.PersistentFlags().BoolVar(&noCacheFlag, "no-cache", false, "Disable disk cache (in-memory deduplication still active)")

	root.AddCommand(NewVersionCmd(version, buildTime))
	root.AddCommand(NewConfigCmd())
	root.AddCommand(NewFlowCmd())
	root.AddCommand(NewQualityCmd())
	root.AddCommand(NewRiskCmd())
	root.AddCommand(NewStatusCmd())
	root.AddCommand(NewReportCmd())

	// Deprecated: keep `release` as a hidden alias for backwards compatibility.
	deprecatedRelease := NewReleaseCmd()
	deprecatedRelease.Hidden = true
	deprecatedRelease.Deprecated = "use 'quality release' instead"
	root.AddCommand(deprecatedRelease)

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

// isRepoAutoDetected returns true when resolveRepo will fall through to
// git remote detection (i.e., no --repo flag and no GH_REPO env var).
func isRepoAutoDetected(repoFlag string) bool {
	return repoFlag == "" && os.Getenv("GH_REPO") == ""
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
