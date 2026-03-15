package cmd

import (
	"github.com/spf13/cobra"
)

// NewRiskCmd returns the risk parent command grouping structural risk signals.
func NewRiskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "risk",
		Short: "Structural risk signals",
		Long: `Where are structural risks in your codebase?

Risk signals identify single points of failure and knowledge
concentration:

  bus-factor    Who knows what? Identifies files and directories where
                knowledge is concentrated in one or two contributors`,
	}
	cmd.AddCommand(NewBusFactorCmd())
	return cmd
}
