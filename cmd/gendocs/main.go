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

	"github.com/bitsbyme/gh-velocity/cmd"
	"github.com/spf13/cobra/doc"
)

const outputDir = "site/content/reference/commands"

func main() {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	root := cmd.NewRootCmd("dev", "")
	root.DisableAutoGenTag = true

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

		return fmt.Sprintf(`---
title: "%s"
bookToC: true
---

`, title)
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
