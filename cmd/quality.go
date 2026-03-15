package cmd

import (
	"github.com/spf13/cobra"
)

// NewQualityCmd returns the quality parent command with release as a subcommand.
func NewQualityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quality",
		Short: "Quality metrics for releases",
		Long: `Is this release good?

Quality metrics analyze your releases:

  release    Composition, defect rate, timing, and per-issue breakdown
             for a specific release tag`,
	}
	cmd.AddCommand(NewReleaseCmd())
	return cmd
}
