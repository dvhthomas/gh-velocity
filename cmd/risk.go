package cmd

import (
	"github.com/spf13/cobra"
)

// NewRiskCmd returns the risk parent command grouping structural risk signals.
func NewRiskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "risk",
		Short: "Structural risk signals",
		Long:  "Structural risk signals about the codebase and team: knowledge concentration, single points of failure.",
	}
	cmd.AddCommand(NewBusFactorCmd())
	return cmd
}
