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

	"github.com/dvhthomas/gh-velocity/internal/config"
	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/gitdata"
	gh "github.com/dvhthomas/gh-velocity/internal/github"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/scope"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/spf13/cobra"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const configKey contextKey = "config"

// OutputConfig controls result format(s) and file destination.
type OutputConfig struct {
	Results      []format.Format // requested output formats (default: [pretty])
	WriteTo      string          // directory for file output; empty = stdout
	SuppressWarn bool            // true when JSON is sole result format going to stdout
}

// Deps holds shared dependencies injected into subcommands.
type Deps struct {
	Config       *config.Config
	Output       OutputConfig
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

// Warn emits a warning to stderr unless SuppressWarn is true.
// When JSON is the sole result format going to stdout, warnings are
// suppressed — they're embedded in the JSON payload instead.
func (d *Deps) Warn(format string, args ...any) {
	if !d.Output.SuppressWarn {
		log.Warn(format, args...)
	}
}

// ResultFormat returns the primary result format (first in the list).
func (d *Deps) ResultFormat() format.Format {
	if len(d.Output.Results) > 0 {
		return d.Output.Results[0]
	}
	return format.Pretty
}

// RenderCtx builds a format.RenderContext from Deps and a writer.
func (d *Deps) RenderCtx(w io.Writer) format.RenderContext {
	return format.RenderContext{
		Writer: w,
		Format: d.ResultFormat(),
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
// error output to stderr when --results json is the sole format, and
// returning the appropriate exit code.
func handleError(root *cobra.Command, err error) int {
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		// Wrap non-AppError into a generic AppError for consistent output.
		appErr = &model.AppError{
			Code:    "INTERNAL",
			Message: err.Error(),
		}
	}

	resultsSlice, _ := root.PersistentFlags().GetStringSlice("results")
	if len(resultsSlice) == 1 && resultsSlice[0] == "json" {
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
		resultsFlag []string
		writeToFlag string
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

			// --post coerces results to markdown unless user explicitly set -r.
			if postFlag && !cmd.Flags().Changed("results") {
				resultsFlag = []string{"markdown"}
			}

			// Validate and normalize result formats.
			results, err := format.ParseResults(resultsFlag)
			if err != nil {
				return err
			}

			// --write-to validation: fail fast before any API calls.
			if writeToFlag != "" {
				for _, f := range results {
					if f == format.Pretty {
						return &model.AppError{
							Code:    model.ErrConfigInvalid,
							Message: "--write-to does not support pretty format (terminal-only)",
						}
					}
				}
				if err := os.MkdirAll(writeToFlag, 0o755); err != nil {
					return &model.AppError{
						Code:    model.ErrConfigInvalid,
						Message: fmt.Sprintf("cannot create --write-to directory %q: %v", writeToFlag, err),
					}
				}
			}

			// Multi-format requires --write-to.
			if len(results) > 1 && writeToFlag == "" {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "multiple --results formats require --write-to <dir>",
				}
			}

			// --post requires markdown in the results list.
			if postFlag {
				hasMarkdown := false
				for _, f := range results {
					if f == format.Markdown {
						hasMarkdown = true
						break
					}
				}
				if !hasMarkdown {
					return &model.AppError{
						Code:    model.ErrConfigInvalid,
						Message: fmt.Sprintf("--post requires markdown in --results (got: %v)", resultsFlag),
					}
				}
			}

			// Suppress warnings on stderr when JSON is the sole result
			// format going to stdout. When --write-to is set, stdout is
			// empty so stderr is always safe — no suppression needed.
			suppressWarn := len(results) == 1 && results[0] == format.JSON && writeToFlag == ""

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

			// Warn if config scope contains a repo: qualifier that conflicts with -R.
			if configRepo, conflict := scope.DetectRepoConflict(cfg.Scope.Query, owner+"/"+repo); conflict {
				log.Warn("config scope includes \"repo:%s\" but -R targets %s/%s — config scope overrides -R flag", configRepo, owner, repo)
			}

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
				log.Debug("results:      %v", resultsFlag)
				if writeToFlag != "" {
					log.Debug("write-to:     %s", writeToFlag)
				}
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
				Config: cfg,
				Output: OutputConfig{
					Results:      results,
					WriteTo:      writeToFlag,
					SuppressWarn: suppressWarn,
				},
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

	root.PersistentFlags().StringSliceVarP(&resultsFlag, "results", "r", []string{"pretty"}, "Output format(s): json, pretty, markdown (comma-separated)")
	root.PersistentFlags().StringVar(&writeToFlag, "write-to", "", "Write result files to this directory (silences stdout)")
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
