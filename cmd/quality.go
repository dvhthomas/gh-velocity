package cmd

import (
	"github.com/spf13/cobra"
)

// NewQualityCmd returns the quality parent command with release as a subcommand.
func NewQualityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quality",
		Short: "Quality metrics for releases",
		Long:  "Quality metrics scoped to releases: defect rate, hotfix detection, category composition.",
	}
	cmd.AddCommand(NewReleaseCmd())
	cmd.AddCommand(NewBusFactorCmd())
	return cmd
}
