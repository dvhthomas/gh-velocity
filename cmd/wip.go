package cmd

import (
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

// NewWIPCmd returns the wip command.
func NewWIPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wip",
		Short: "Show work in progress",
		Long: `Show items currently in progress.

Primary source: Projects v2 board status (requires project.id in config).
Fallback: open issues with active_labels (requires statuses.active_labels in config).

Use -R owner/repo to filter board items to a specific repo.`,
		Example: `  # Show WIP from configured project board
  gh velocity status wip

  # Filter to a specific repo on the board
  gh velocity status wip -R owner/repo

  # JSON output for CI/automation
  gh velocity status wip -f json`,
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

			client, err := gh.NewClient(deps.Owner, deps.Repo)
			if err != nil {
				return err
			}

			now := time.Now().UTC()
			cfg := deps.Config
			repo := deps.Owner + "/" + deps.Repo

			var items []model.WIPItem

			if cfg.Project.ID != "" {
				// Primary: Projects v2 board
				projectItems, listErr := client.ListProjectItems(ctx, cfg.Project.ID, cfg.Project.StatusFieldID)
				if listErr != nil {
					return listErr
				}

				for _, pi := range projectItems {
					// Filter out backlog and done items.
					if pi.Status == cfg.Statuses.Backlog || pi.Status == cfg.Statuses.Done {
						continue
					}

					// Filter by repo if -R is given.
					if pi.Repo != "" && pi.Repo != repo {
						continue
					}

					age := now.Sub(pi.CreatedAt)
					if pi.StatusAt != nil {
						age = now.Sub(*pi.StatusAt)
					}

					items = append(items, model.WIPItem{
						Number: pi.Number,
						Title:  pi.Title,
						Status: pi.Status,
						Age:    age,
						Repo:   pi.Repo,
						Kind:   pi.ContentType,
					})
				}
			} else if len(cfg.Statuses.ActiveLabels) > 0 {
				// Fallback: label-based WIP
				activeIssues, searchErr := client.SearchOpenIssuesWithLabels(ctx, cfg.Statuses.ActiveLabels)
				if searchErr != nil {
					return searchErr
				}

				backlogSet := make(map[string]bool)
				for _, l := range cfg.Statuses.BacklogLabels {
					backlogSet[l] = true
				}

				for _, issue := range activeIssues {
					// Exclude issues with backlog labels.
					hasBacklog := false
					for _, l := range issue.Labels {
						if backlogSet[l] {
							hasBacklog = true
							break
						}
					}
					if hasBacklog {
						continue
					}

					age := now.Sub(issue.CreatedAt)
					items = append(items, model.WIPItem{
						Number: issue.Number,
						Title:  issue.Title,
						Status: "active",
						Age:    age,
						Kind:   "Issue",
					})
				}
			} else {
				return &model.AppError{
					Code:    model.ErrConfigInvalid,
					Message: "wip requires either project.id or statuses.active_labels in .gh-velocity.yml\n\n  To auto-detect your setup:  gh velocity config preflight -R owner/repo\n  To find project board IDs:  gh velocity config discover -R owner/repo",
				}
			}

			w := cmd.OutOrStdout()
			switch deps.Format {
			case format.JSON:
				return format.WriteWIPJSON(w, repo, items)
			case format.Markdown:
				return format.WriteWIPMarkdown(w, repo, items)
			default:
				return format.WriteWIPPretty(w, deps.IsTTY, deps.TermWidth, repo, items)
			}
		},
	}

	return cmd
}
