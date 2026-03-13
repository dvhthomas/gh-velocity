package cmd

import (
	"github.com/spf13/cobra"
)

// NewFlowCmd returns the flow parent command grouping velocity metrics.
func NewFlowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flow",
		Short: "Flow metrics (lead-time, cycle-time, velocity)",
		Long:  "Velocity and throughput metrics: how fast is work flowing?",
	}
	cmd.AddCommand(NewLeadTimeCmd())
	cmd.AddCommand(NewCycleTimeCmd())
	cmd.AddCommand(NewThroughputCmd())
	cmd.AddCommand(NewVelocityCmd())
	return cmd
}
