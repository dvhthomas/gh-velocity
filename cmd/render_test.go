package cmd

import (
	"testing"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/spf13/cobra"
)

func TestCommandSlug(t *testing.T) {
	tests := []struct {
		name string
		path string // simulated CommandPath
		want string
	}{
		{"root command", "gh-velocity report", "report"},
		{"nested", "gh-velocity flow lead-time", "flow-lead-time"},
		{"deep nested", "gh-velocity quality release", "quality-release"},
		{"single", "gh-velocity", "gh-velocity"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a command tree to get the right CommandPath.
			parts := splitPath(tt.path)
			cmd := buildCmdTree(parts)
			got := commandSlug(cmd)
			if got != tt.want {
				t.Errorf("commandSlug() = %q, want %q (path=%q)", got, tt.want, cmd.CommandPath())
			}
		})
	}
}

// splitPath splits a space-separated path into command names.
func splitPath(s string) []string {
	var parts []string
	for _, p := range []byte(s) {
		if p == ' ' {
			parts = append(parts, "")
		}
	}
	// Simple split.
	result := make([]string, 0)
	current := ""
	for _, c := range s {
		if c == ' ' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// buildCmdTree builds a command tree and returns the leaf command.
func buildCmdTree(names []string) *cobra.Command {
	if len(names) == 0 {
		return &cobra.Command{Use: "test"}
	}

	root := &cobra.Command{Use: names[0]}
	current := root
	for _, name := range names[1:] {
		child := &cobra.Command{Use: name}
		current.AddCommand(child)
		current = child
	}
	return current
}

func TestFormatExt(t *testing.T) {
	tests := []struct {
		name string
		f    string
		want string
	}{
		{"json", "json", "json"},
		{"markdown", "markdown", "md"},
		{"html", "html", "html"},
		{"pretty", "pretty", "txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExt(format.Format(tt.f))
			if got != tt.want {
				t.Errorf("formatExt(%q) = %q, want %q", tt.f, got, tt.want)
			}
		})
	}
}
