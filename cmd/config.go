package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dvhthomas/gh-velocity/internal/config"
	gh "github.com/dvhthomas/gh-velocity/internal/github"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

// NewConfigCmd returns the config parent command with show and validate subcommands.
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect, validate, and generate configuration",
		Long: `Configuration commands for .gh-velocity.yml.

Getting started:
  1. gh velocity config preflight -R owner/repo  # analyze repo, suggest config
  2. gh velocity config create                    # generate starter config
  3. gh velocity config validate                  # check for errors`,
	}

	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigValidateCmd())
	cmd.AddCommand(newConfigCreateCmd())
	cmd.AddCommand(newConfigDiscoverCmd())
	cmd.AddCommand(newConfigPreflightCmd())

	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display resolved configuration with defaults applied",
		Long: `Display the fully resolved configuration with all defaults applied.

This shows the effective config that commands will use — including default
values for any keys you have not set in your .gh-velocity.yml. Use this
to verify that your config is interpreted the way you expect.

The JSON output (-f json) is useful for debugging or piping into other tools.`,
		Example: `  gh velocity config show
  gh velocity config show -f json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolveConfigPath(cmd))
			if err != nil {
				return emitConfigError(cmd, err)
			}

			formatFlag, _ := cmd.Flags().GetString("format")
			if formatFlag == "json" {
				out, err := json.MarshalIndent(cfg, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			// Pretty-print as key-value pairs.
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "workflow:                    %s\n", cfg.Workflow)
			fmt.Fprintf(w, "scope.query:                 %s\n", cfg.Scope.Query)
			fmt.Fprintf(w, "project.url:                 %s\n", cfg.Project.URL)
			fmt.Fprintf(w, "project.status_field:        %s\n", cfg.Project.StatusField)
			fmt.Fprintf(w, "lifecycle.backlog.query:      %s\n", cfg.Lifecycle.Backlog.Query)
			if len(cfg.Lifecycle.Backlog.Match) > 0 {
				fmt.Fprintf(w, "lifecycle.backlog.match:      %v\n", cfg.Lifecycle.Backlog.Match)
			}
			fmt.Fprintf(w, "lifecycle.in-progress.query:  %s\n", cfg.Lifecycle.InProgress.Query)
			if len(cfg.Lifecycle.InProgress.Match) > 0 {
				fmt.Fprintf(w, "lifecycle.in-progress.match:  %v\n", cfg.Lifecycle.InProgress.Match)
			}
			fmt.Fprintf(w, "lifecycle.in-review.query:    %s\n", cfg.Lifecycle.InReview.Query)
			fmt.Fprintf(w, "lifecycle.done.query:         %s\n", cfg.Lifecycle.Done.Query)
			fmt.Fprintf(w, "quality.hotfix_window_hours:  %g\n", cfg.Quality.HotfixWindowHours)
			fmt.Fprintf(w, "quality.categories:\n")
			for _, cat := range cfg.Quality.Categories {
				fmt.Fprintf(w, "  - %s: %v\n", cat.Name, cat.Matchers)
			}
			fmt.Fprintf(w, "discussions.category:        %s\n", cfg.Discussions.Category)
			fmt.Fprintf(w, "cycle_time.strategy:         %s\n", cfg.CycleTime.Strategy)
			fmt.Fprintf(w, "velocity.unit:               %s\n", cfg.Velocity.Unit)
			fmt.Fprintf(w, "velocity.effort.strategy:    %s\n", cfg.Velocity.Effort.Strategy)
			if cfg.Velocity.Effort.Strategy == "attribute" {
				for i, m := range cfg.Velocity.Effort.Attribute {
					fmt.Fprintf(w, "  attribute[%d]: %s → %.0f\n", i, m.Query, m.Value)
				}
			}
			if cfg.Velocity.Effort.Strategy == "numeric" {
				fmt.Fprintf(w, "  numeric.project_field:     %s\n", cfg.Velocity.Effort.Numeric.ProjectField)
			}
			fmt.Fprintf(w, "velocity.iteration.strategy: %s\n", cfg.Velocity.Iteration.Strategy)
			if cfg.Velocity.Iteration.Strategy == "project-field" {
				fmt.Fprintf(w, "  project_field:             %s\n", cfg.Velocity.Iteration.ProjectField)
			}
			if cfg.Velocity.Iteration.Strategy == "fixed" {
				fmt.Fprintf(w, "  fixed.length:              %s\n", cfg.Velocity.Iteration.Fixed.Length)
				fmt.Fprintf(w, "  fixed.anchor:              %s\n", cfg.Velocity.Iteration.Fixed.Anchor)
			}
			fmt.Fprintf(w, "velocity.iteration.count:    %d\n", cfg.Velocity.Iteration.Count)
			if len(cfg.ExcludeUsers) > 0 {
				fmt.Fprintf(w, "exclude_users:               %v\n", cfg.ExcludeUsers)
			}
			return nil
		},
	}
}

func newConfigValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration and report errors",
		Long: `Validate a .gh-velocity.yml file and report any errors.

Checks include: YAML syntax, known top-level keys, matcher syntax
(label:, type:, title:, field: patterns), project URL format, numeric
ranges (e.g., hotfix_window_hours), and category name requirements.

Use --velocity to additionally verify that velocity-specific config
(effort strategy, iteration strategy, project field names) is correctly
configured against your actual GitHub project board.

This command does not make API calls (except with --velocity, which
queries the project board to validate field names).`,
		Example: `  gh velocity config validate
  gh velocity config validate --velocity`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolveConfigPath(cmd))
			if err != nil {
				return emitConfigError(cmd, err)
			}

			formatFlag, _ := cmd.Flags().GetString("format")
			if formatFlag == "json" {
				out, _ := json.MarshalIndent(map[string]string{
					"status": "valid",
				}, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "config: valid")
			}

			// Run velocity-specific live validation if --velocity flag is set.
			if _, err := runVelocityValidation(cmd, cfg); err != nil {
				return err
			}

			return nil
		},
	}

	addVelocityValidateFlag(cmd)
	return cmd
}

const defaultConfigTemplate = `# gh-velocity configuration
# See: https://github.com/dvhthomas/gh-velocity/blob/main/docs/guide.md

# Scope: which issues/PRs to analyze (GitHub search query syntax).
# Build your query at github.com/issues, paste it here.
# scope:
#   query: 'repo:myorg/myrepo label:"bug"'

# Issue/PR classification — first matching category wins; unmatched = "other".
# Matchers: label:<name>, type:<name>, title:/<regex>/i
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
    - name: feature
      match:
        - "label:enhancement"
  hotfix_window_hours: 72

# Commit message scanning
# "closes" matches: fixes #N, closes #N, resolves #N (default)
# "refs" also matches bare #N references (more aggressive)
commit_ref:
  patterns: ["closes"]

# Cycle time strategy: "issue" (default) or "pr"
# Issue strategy uses lifecycle.in-progress to detect "work started".
# cycle_time:
#   strategy: issue

# GitHub Projects v2 board (uncomment to enable)
# CI/Actions: GITHUB_TOKEN cannot access projects. Create a classic PAT with
# 'project' scope: https://github.com/settings/tokens/new?scopes=project&description=gh-velocity
# project:
#   url: "https://github.com/users/yourname/projects/1"
#   status_field: "Status"

# Lifecycle stages: map labels and/or project board columns to workflow stages.
# Used by cycle time to detect "work started" and by WIP to filter board items.
#
# match: recommended for cycle time — label timestamps are immutable and reliable.
# project_status: used for WIP detection and backlog filtering (board column names).
#
# Example with both (recommended when using a project board):
# lifecycle:
#   backlog:
#     project_status: ["Backlog", "Triage"]
#   in-progress:
#     project_status: ["In progress"]
#     match: ["label:in-progress", "label:wip"]
#   done:
#     project_status: ["Done", "Shipped"]
#
# Example with labels only (no project board):
# lifecycle:
#   backlog:
#     match: ["label:backlog"]
#   in-progress:
#     match: ["label:in-progress", "label:wip"]

# Exclude bot accounts from metrics (e.g., dependabot, renovate).
# These are filtered via -author: qualifiers in search queries.
# exclude_users:
#   - "dependabot[bot]"
#   - "renovate[bot]"

# Minimum seconds between GitHub search API calls.
# Prevents secondary (abuse) rate limits which trigger on burst traffic.
# Set to 0 to disable (not recommended for CI).
# api_throttle_seconds: 2

# Velocity: effort completed per iteration (sprint velocity).
# velocity:
#   unit: issues                    # "issues" (default) or "prs"
#   effort:
#     strategy: count               # "count" (default), "attribute", or "numeric"
#     # attribute strategy — map labels/types/fields to effort values (first match wins):
#     # attribute:
#     #   - query: "label:size/XS"
#     #     value: 1
#     #   - query: "label:size/S"
#     #     value: 2
#     #   - query: "label:size/M"
#     #     value: 3
#     #   - query: "label:size/L"
#     #     value: 5
#     #   - query: "label:size/XL"
#     #     value: 8
#     #   # field: matchers use project board SingleSelect fields:
#     #   # - query: "field:Size/S"
#     #   #   value: 2
#     # numeric strategy — read effort from a project board Number field:
#     # numeric:
#     #   project_field: "Story Points"
#   iteration:
#     strategy: fixed               # "project-field" or "fixed"
#     # project_field: "Sprint"     # name of ProjectV2 Iteration field
#     fixed:
#       length: "14d"               # iteration length (e.g., "14d", "1w")
#       anchor: "2026-01-06"        # start date of any iteration
#     count: 6                      # number of past iterations to show

# Post results to GitHub (--post flag).
# CI/Actions: requires 'issues: write' for issue/PR comments,
# 'discussions: write' for bulk reports.
# discussions:
#   category: General
`

func newConfigCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create a default .gh-velocity.yml in the current directory",
		Long: `Generate a starter .gh-velocity.yml with sensible defaults.

For a smarter config tailored to your repo, use 'config preflight' first:
  gh velocity config preflight -R owner/repo`,
		Example: `  gh velocity config create`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := config.DefaultConfigFile
			if _, err := os.Stat(path); err == nil {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: fmt.Sprintf("%s already exists; remove it first or edit it directly", path),
				}
			}
			if err := os.WriteFile(path, []byte(defaultConfigTemplate), 0644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", path)
			return nil
		},
	}
}

func newConfigDiscoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "Discover Projects v2 boards and fields linked to a repo",
		Long: `Queries the GitHub API to find Projects v2 boards linked to the target
repository, then lists their fields and status options.

Use this to find the project URL and status field name
needed for .gh-velocity.yml configuration.`,
		Example: `  # Discover projects for a remote repo
  gh velocity config discover -R cli/cli

  # JSON output for scripting
  gh velocity config discover -R owner/repo -f json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Config subcommands skip PersistentPreRunE, so resolve repo here.
			repoFlag, _ := cmd.Root().PersistentFlags().GetString("repo")
			owner, repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}

			// Detect auto-detection from git remote.
			if isRepoAutoDetected(repoFlag) {
				log.Notice("Using repo %s/%s from git remote (use --repo to override)", owner, repo)
			}

			client, err := gh.NewClient(owner, repo, 0)
			if err != nil {
				return err
			}

			projects, err := client.DiscoverProjects(cmd.Context())
			if err != nil {
				return err
			}

			if len(projects) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No Projects v2 boards linked to %s/%s.\n", owner, repo)
				return nil
			}

			formatFlag, _ := cmd.Root().PersistentFlags().GetString("format")
			if formatFlag == "json" {
				out, err := json.MarshalIndent(projects, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Projects linked to %s/%s\n", owner, repo)
			fmt.Fprintln(w, strings.Repeat("=", 60))

			for _, p := range projects {
				fmt.Fprintf(w, "\nProject: %s (#%d)\n", p.Title, p.Number)
				fmt.Fprintf(w, "  id: %s\n", p.ID)

				// Find and highlight the Status field.
				for _, f := range p.Fields {
					if len(f.Options) == 0 {
						continue
					}
					marker := " "
					if strings.EqualFold(f.Name, "Status") {
						marker = "*"
					}
					fmt.Fprintf(w, "\n %s Field: %s\n", marker, f.Name)
					fmt.Fprintf(w, "   id: %s\n", f.ID)
					fmt.Fprintf(w, "   Options:\n")
					for _, o := range f.Options {
						fmt.Fprintf(w, "     - %s\n", o.Name)
					}
				}

				// Print config snippet for Status field.
				for _, f := range p.Fields {
					if strings.EqualFold(f.Name, "Status") && len(f.Options) > 0 {
						fmt.Fprintf(w, "\n  Config snippet for .gh-velocity.yml:\n")
						fmt.Fprintf(w, "    project:\n")
						fmt.Fprintf(w, "      url: %q  # internal id: %s\n", p.URL, p.ID)
						fmt.Fprintf(w, "      status_field: %q  # internal id: %s\n", f.Name, f.ID)
						break
					}
				}
			}

			return nil
		},
	}
}

// resolveConfigPath returns the config file path from --config flag or default.
func resolveConfigPath(cmd *cobra.Command) string {
	if f, _ := cmd.Root().PersistentFlags().GetString("config"); f != "" {
		return f
	}
	return config.DefaultConfigFile
}

// emitConfigError wraps a config loading error into a structured AppError.
func emitConfigError(cmd *cobra.Command, err error) error {
	formatFlag, _ := cmd.Flags().GetString("format")
	appErr := &model.AppError{
		Code:    model.ErrConfigInvalid,
		Message: err.Error(),
	}
	if formatFlag == "json" {
		envelope := &model.ErrorEnvelope{Error: appErr}
		out, _ := envelope.JSON()
		fmt.Fprintln(cmd.OutOrStdout(), string(out))
		// Return nil so cobra doesn't print the error again;
		// the structured output has been written.
		return nil
	}
	return appErr
}
