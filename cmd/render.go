package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvhthomas/gh-velocity/internal/format"
	gh "github.com/dvhthomas/gh-velocity/internal/github"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/pipeline"
	"github.com/dvhthomas/gh-velocity/internal/posting"
	"github.com/spf13/cobra"
)

// renderPipeline runs a Pipeline through all requested result formats,
// handles --write-to file routing, and posts if --post is set.
func renderPipeline(cmd *cobra.Command, deps *Deps, p pipeline.Pipeline, client *gh.Client, postOpts posting.PostOptions) error {
	pc, postFn := setupPost(cmd, deps, client, postOpts)

	if deps.Output.WriteTo == "" {
		// Single-format to stdout.
		stdout := cmd.OutOrStdout()
		var w = stdout
		if pc != nil {
			w = pc.postWriter(stdout)
		}
		rc := deps.RenderCtx(w)
		if err := p.Render(rc); err != nil {
			return err
		}
		return postFn()
	}

	// Multi-format to files. Also render markdown to post buffer if posting.
	if pc != nil {
		rc := format.RenderContext{
			Writer: &pc.buf,
			Format: format.Markdown,
			IsTTY:  false,
			Width:  deps.TermWidth,
			Owner:  deps.Owner,
			Repo:   deps.Repo,
		}
		if err := p.Render(rc); err != nil {
			return err
		}
	}

	slug := commandSlug(cmd)
	var written []string
	for _, f := range deps.Output.Results {
		name := slug + "." + formatExt(f)
		path := filepath.Join(deps.Output.WriteTo, name)

		if err := writeFileAtomic(path, func(w *os.File) error {
			rc := deps.RenderCtx(w)
			rc.Format = f
			return p.Render(rc)
		}); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		written = append(written, name)
	}
	log.Debug("artifacts written to %s (%s)", deps.Output.WriteTo, strings.Join(written, ", "))
	return postFn()
}

// renderPipelineSimple runs a Pipeline through all requested result formats
// without post support. Use for commands that don't support --post.
func renderPipelineSimple(cmd *cobra.Command, deps *Deps, p pipeline.Pipeline) error {
	if deps.Output.WriteTo == "" {
		rc := deps.RenderCtx(cmd.OutOrStdout())
		return p.Render(rc)
	}

	slug := commandSlug(cmd)
	var written []string
	for _, f := range deps.Output.Results {
		name := slug + "." + formatExt(f)
		path := filepath.Join(deps.Output.WriteTo, name)

		if err := writeFileAtomic(path, func(w *os.File) error {
			rc := deps.RenderCtx(w)
			rc.Format = f
			return p.Render(rc)
		}); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		written = append(written, name)
	}
	log.Debug("artifacts written to %s (%s)", deps.Output.WriteTo, strings.Join(written, ", "))
	return nil
}

// commandSlug derives a file-name stem from the command's path.
// "gh-velocity flow lead-time" → "flow-lead-time"
// "gh-velocity report" → "report"
func commandSlug(cmd *cobra.Command) string {
	parts := strings.Fields(cmd.CommandPath())
	if len(parts) > 1 {
		parts = parts[1:] // strip root command name
	}
	return strings.Join(parts, "-")
}

// formatExt returns the file extension for a format.
func formatExt(f format.Format) string {
	switch f {
	case format.JSON:
		return "json"
	case format.Markdown:
		return "md"
	default:
		return "txt"
	}
}

// writeFileAtomic writes to a temp file then renames for atomicity.
func writeFileAtomic(path string, fn func(w *os.File) error) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".gh-velocity-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if err := fn(tmp); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
