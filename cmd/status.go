package cmd

import (
	"github.com/spf13/cobra"
)

// NewStatusCmd returns the status parent command grouping current-state views.
func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Current work status (wip, my-week, reviews)",
		Long: `What is happening right now?

Status commands show the current state of work:

  wip         Work in progress — open items on your project board
  my-week     Personal weekly summary for 1:1 prep
  reviews     Pending and stale pull request reviews`,
	}
	cmd.AddCommand(NewWIPCmd())
	cmd.AddCommand(NewMyWeekCmd())
	cmd.AddCommand(NewReviewsCmd())
	return cmd
}
