package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/bitsbyme/gh-velocity/internal/config"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

// NewConfigCmd returns the config parent command with show and validate subcommands.
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and validate configuration",
	}

	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigValidateCmd())
	cmd.AddCommand(newConfigCreateCmd())
	cmd.AddCommand(newConfigDiscoverCmd())

	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display resolved configuration with defaults applied",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.DefaultConfigFile)
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
			fmt.Fprintf(w, "project.id:                  %s\n", cfg.Project.ID)
			fmt.Fprintf(w, "project.status_field_id:     %s\n", cfg.Project.StatusFieldID)
			fmt.Fprintf(w, "statuses.backlog:            %s\n", cfg.Statuses.Backlog)
			fmt.Fprintf(w, "statuses.ready:              %s\n", cfg.Statuses.Ready)
			fmt.Fprintf(w, "statuses.in_progress:        %s\n", cfg.Statuses.InProgress)
			fmt.Fprintf(w, "statuses.in_review:          %s\n", cfg.Statuses.InReview)
			fmt.Fprintf(w, "statuses.done:               %s\n", cfg.Statuses.Done)
			fmt.Fprintf(w, "fields.start_date:           %s\n", cfg.Fields.StartDate)
			fmt.Fprintf(w, "fields.target_date:          %s\n", cfg.Fields.TargetDate)
			fmt.Fprintf(w, "quality.bug_labels:          %v\n", cfg.Quality.BugLabels)
			fmt.Fprintf(w, "quality.feature_labels:      %v\n", cfg.Quality.FeatureLabels)
			fmt.Fprintf(w, "quality.hotfix_window_hours:  %g\n", cfg.Quality.HotfixWindowHours)
			if len(cfg.Quality.Categories) > 0 {
				fmt.Fprintf(w, "quality.categories:\n")
				for _, cat := range cfg.Quality.Categories {
					fmt.Fprintf(w, "  - %s: %v\n", cat.Name, cat.Matchers)
				}
			}
			fmt.Fprintf(w, "discussions.category_id:     %s\n", cfg.Discussions.CategoryID)
			fmt.Fprintf(w, "cycle_time.strategy:         %s\n", cfg.CycleTime.Strategy)
			return nil
		},
	}
}

func newConfigValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration and report errors",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := config.Load(config.DefaultConfigFile)
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
			return nil
		},
	}
}

const defaultConfigTemplate = `# gh-velocity configuration
# See: https://github.com/dvhthomas/gh-velocity/blob/main/docs/guide.md

# Issue classification labels
quality:
  bug_labels: ["bug"]
  feature_labels: ["enhancement"]
  hotfix_window_hours: 72

# Commit message scanning
# "closes" matches: fixes #N, closes #N, resolves #N (default)
# "refs" also matches bare #N references (more aggressive)
commit_ref:
  patterns: ["closes"]

# Cycle time strategy: "issue" (default), "pr", or "project-board"
# cycle_time:
#   strategy: issue

# Cycle time status signals (uncomment to enable)
# Option A: GitHub Projects v2 board
# project:
#   id: "PVT_kwDOAbc123"
#   status_field_id: "PVTSSF_kwDOAbc123"

# Option B: Label-based status (common in OSS)
# statuses:
#   active_labels: ["in-progress", "wip"]     # labels = work started
#   backlog_labels: ["backlog", "icebox"]      # labels = work not started
`

func newConfigCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create a default .gh-velocity.yml in the current directory",
		Args:  cobra.NoArgs,
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

Use this to find the project.id and project.status_field_id values
needed for .gh-velocity.yml configuration.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Config subcommands skip PersistentPreRunE, so resolve repo here.
			repoFlag, _ := cmd.Root().PersistentFlags().GetString("repo")
			owner, repo, err := resolveRepo(repoFlag)
			if err != nil {
				return err
			}

			client, err := gh.NewClient(owner, repo)
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
						fmt.Fprintf(w, "      id: %q\n", p.ID)
						fmt.Fprintf(w, "      status_field_id: %q\n", f.ID)
						break
					}
				}
			}

			return nil
		},
	}
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
