package cmd

import (
	"github.com/spf13/cobra"
)

// NewFlowCmd returns the flow parent command grouping velocity metrics.
func NewFlowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flow",
		Short: "Flow metrics (lead-time, cycle-time, velocity)",
		Long: `How fast is work flowing through your team?

Flow metrics measure the speed and volume of work:

  lead-time     Total time from issue creation to closure
  cycle-time    Active work time (start signal to close/merge)
  throughput    Items completed per time window
  velocity      Effort delivered per iteration`,
	}
	cmd.AddCommand(NewLeadTimeCmd())
	cmd.AddCommand(NewCycleTimeCmd())
	cmd.AddCommand(NewThroughputCmd())
	cmd.AddCommand(NewVelocityCmd())
	return cmd
}
