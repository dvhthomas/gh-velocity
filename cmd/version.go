package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// NewVersionCmd returns the version command.
func NewVersionCmd(version, buildTime string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			formatFlag, _ := cmd.Flags().GetString("format")
			if formatFlag == "json" {
				out, err := json.Marshal(map[string]string{
					"version":    version,
					"build_time": buildTime,
				})
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "gh-velocity %s (built %s)\n", version, buildTime)
			return nil
		},
	}
}
