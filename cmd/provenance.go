package cmd

import (
	"fmt"
	"strings"

	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// buildProvenance captures the full command invocation for reproducibility.
// It walks explicitly-set flags (both local and inherited) and reconstructs
// the command line. The optional configMap adds key config values that
// affect interpretation of the output.
func buildProvenance(cmd *cobra.Command, configMap map[string]string) model.Provenance {
	seen := map[string]bool{"repo": true} // --repo is context, captured in config
	var parts []string
	parts = append(parts, "gh velocity "+cmd.CommandPath()[len("gh-velocity "):])

	addFlag := func(f *pflag.Flag) {
		if seen[f.Name] {
			return
		}
		seen[f.Name] = true
		switch f.Value.Type() {
		case "bool":
			parts = append(parts, "--"+f.Name)
		default:
			parts = append(parts, fmt.Sprintf("--%s %s", f.Name, f.Value.String()))
		}
	}
	cmd.Flags().Visit(addFlag)
	cmd.InheritedFlags().Visit(addFlag)

	return model.Provenance{
		Command: strings.Join(parts, " "),
		Config:  configMap,
	}
}
