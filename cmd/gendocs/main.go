// Command gendocs generates CLI reference documentation from Cobra command
// definitions. Output is Hugo-compatible markdown with front matter for the
// Hugo Book theme.
//
// Usage: go run ./cmd/gendocs
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvhthomas/gh-velocity/cmd"
	cobracmd "github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

const outputDir = "site/content/reference/commands"

func main() {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	root := cmd.NewRootCmd("dev", "")
	root.DisableAutoGenTag = true

	// Build a map of filename → Short description for front matter.
	descriptions := map[string]string{}
	buildDescriptions(root, descriptions)

	// filePrepender injects Hugo front matter into each generated page.
	filePrepender := func(filename string) string {
		name := filepath.Base(filename)
		name = strings.TrimSuffix(name, filepath.Ext(name))
		// Convert "gh-velocity_flow_velocity" to "flow velocity"
		title := strings.ReplaceAll(name, "gh-velocity_", "")
		title = strings.ReplaceAll(title, "_", " ")
		if title == "gh-velocity" {
			title = "gh velocity"
		}

		desc := descriptions[name]
		if desc != "" {
			// Use single-quoted YAML so backticks in descriptions are preserved.
			return fmt.Sprintf("---\ntitle: \"%s\"\ndescription: '%s'\nbookToC: true\n---\n\n", title, strings.ReplaceAll(desc, "'", "''"))
		}
		return fmt.Sprintf("---\ntitle: \"%s\"\nbookToC: true\n---\n\n", title)
	}

	// linkHandler rewrites links to work with Hugo's URL scheme.
	linkHandler := func(name string) string {
		base := strings.TrimSuffix(name, filepath.Ext(name))
		return "/gh-velocity/reference/commands/" + strings.ToLower(base) + "/"
	}

	if err := doc.GenMarkdownTreeCustom(root, outputDir, filePrepender, linkHandler); err != nil {
		log.Fatalf("generate docs: %v", err)
	}

	// Count generated files.
	entries, _ := os.ReadDir(outputDir)
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && e.Name() != "_index.md" {
			count++
		}
	}
	fmt.Printf("Generated %d command reference pages in %s/\n", count, outputDir)
}

// buildDescriptions walks the command tree and maps cobra/doc filenames to Short descriptions.
func buildDescriptions(cmd *cobracmd.Command, m map[string]string) {
	name := strings.ReplaceAll(cmd.CommandPath(), " ", "_")
	if cmd.Short != "" {
		m[name] = cmd.Short
	}
	for _, sub := range cmd.Commands() {
		buildDescriptions(sub, m)
	}
}
