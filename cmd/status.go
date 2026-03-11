package cmd

import (
	"github.com/spf13/cobra"
)

// NewStatusCmd returns the status parent command grouping current-state views.
func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Current work status (wip)",
		Long:  "What is happening right now? In-progress work and aging.",
	}
	cmd.AddCommand(NewWIPCmd())
	cmd.AddCommand(NewMyWeekCmd())
	return cmd
}
