package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/spf13/cobra"
)

// NewWIPCmd returns the wip command.
func NewWIPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wip",
		Short: "Show work in progress",
		Long: `Show items currently in progress.

Primary source: Projects v2 board status (requires project.url in config).

Use -R owner/repo to filter board items to a specific repo.`,
		Example: `  # Show WIP from configured project board
  gh velocity status wip

  # Filter to a specific repo on the board
  gh velocity status wip -R owner/repo

  # JSON output for CI/automation
  gh velocity status wip -f json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWIP(cmd)
		},
	}

	return cmd
}

func runWIP(cmd *cobra.Command) error {
	ctx := cmd.Context()
	deps := DepsFromContext(ctx)
	if deps == nil {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "internal error: missing dependencies",
		}
	}

	cfg := deps.Config

	if cfg.Project.URL == "" {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "wip requires project.url in .gh-velocity.yml\n\n  To auto-detect your setup:  gh velocity config preflight -R owner/repo\n  To find project boards:     gh velocity config discover -R owner/repo",
		}
	}

	client, err := deps.NewClient()
	if err != nil {
		return err
	}

	// Resolve project URL → node IDs.
	info, err := client.ResolveProject(ctx, cfg.Project.URL, cfg.Project.StatusField)
	if err != nil {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: fmt.Sprintf("resolve project board: %v", err),
		}
	}

	// Fetch all items from the board.
	items, err := client.ListProjectItems(ctx, info.ProjectID, info.StatusFieldID)
	if err != nil {
		return err
	}

	// Build set of WIP statuses from lifecycle config.
	wipStatuses := make(map[string]bool)
	for _, s := range cfg.Lifecycle.InProgress.ProjectStatus {
		wipStatuses[s] = true
	}
	for _, s := range cfg.Lifecycle.InReview.ProjectStatus {
		wipStatuses[s] = true
	}

	if len(wipStatuses) == 0 {
		return &model.AppError{
			Code:    model.ErrConfigInvalid,
			Message: "no lifecycle stages define project_status for in-progress or in-review\n\n  Run: gh velocity config preflight -R owner/repo --write",
		}
	}

	// Filter to WIP items and convert to display type.
	now := deps.Now()
	repoFilter := ""
	if deps.Owner != "" && deps.Repo != "" {
		repoFilter = deps.Owner + "/" + deps.Repo
	}

	var wipItems []model.WIPItem
	for _, item := range items {
		if !wipStatuses[item.Status] {
			continue
		}
		// Filter by repo if -R was set.
		if repoFilter != "" && item.Repo != repoFilter {
			continue
		}

		wipItems = append(wipItems, toWIPItem(item, now))
	}

	// Output.
	repo := fmt.Sprintf("%s/%s", deps.Owner, deps.Repo)
	rc := deps.RenderCtx(os.Stdout)

	switch deps.Format {
	case format.JSON:
		return format.WriteWIPJSON(os.Stdout, repo, wipItems)
	case format.Markdown:
		return format.WriteWIPMarkdown(rc, repo, wipItems)
	default:
		return format.WriteWIPPretty(rc, repo, wipItems)
	}
}

// toWIPItem converts a ProjectItem to a display-ready WIPItem.
func toWIPItem(item model.ProjectItem, now time.Time) model.WIPItem {
	age := now.Sub(item.CreatedAt)

	staleness := model.StalenessActive
	sinceUpdate := now.Sub(item.UpdatedAt)
	switch {
	case sinceUpdate > 7*24*time.Hour:
		staleness = model.StalenessStale
	case sinceUpdate > 3*24*time.Hour:
		staleness = model.StalenessAging
	}

	return model.WIPItem{
		Number:    item.Number,
		Title:     item.Title,
		Status:    item.Status,
		Age:       age,
		Repo:      item.Repo,
		Kind:      item.ContentType,
		URL:       item.URL,
		Labels:    item.Labels,
		UpdatedAt: item.UpdatedAt,
		Staleness: staleness,
	}
}
