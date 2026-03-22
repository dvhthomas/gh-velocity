package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvhthomas/gh-velocity/internal/format"
	gh "github.com/dvhthomas/gh-velocity/internal/github"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/pipeline"
	"github.com/dvhthomas/gh-velocity/internal/posting"
	"github.com/spf13/cobra"
)

// renderPipeline runs a Pipeline through the full lifecycle:
// GatherData → [Enrich] → ProcessData → Warn → Render → Post.
// Standalone commands should ALWAYS use this — never call phases directly.
// The report command is the one exception (see cmd/report.go).
// Pass nil for client and empty PostOptions for commands without --post.
func renderPipeline(cmd *cobra.Command, deps *Deps, p pipeline.Pipeline, client *gh.Client, postOpts posting.PostOptions) error {
	ctx := cmd.Context()

	// Phase 1: Gather data from APIs.
	if err := p.GatherData(ctx); err != nil {
		return err
	}
	// Phase 1.5: Optional enrichment (e.g., IssueType) between gather and process.
	if e, ok := p.(pipeline.Enricher); ok {
		if err := e.Enrich(ctx); err != nil {
			return err
		}
	}
	// Phase 2: Compute metrics from gathered data.
	if err := p.ProcessData(); err != nil {
		return err
	}
	// Surface warnings.
	for _, w := range p.Warnings() {
		deps.Warn("%s", w)
	}

	// Phase 3: Render in requested format(s).

	// Skip generic provenance for pipelines that render their own (e.g., velocity).
	_, hasOwnProvenance := p.(provenanceRenderer)
	var prov model.Provenance
	if !hasOwnProvenance {
		prov = buildProvenance(cmd, map[string]string{
			"repository": deps.Owner + "/" + deps.Repo,
		})
	}

	pc, postFn := setupPost(cmd, deps, client, postOpts)

	if deps.Output.WriteTo == "" {
		// Single-format to stdout.
		stdout := cmd.OutOrStdout()
		var w = stdout
		if pc != nil {
			w = pc.postWriter(stdout)
		}
		f := deps.ResultFormat()
		if err := renderFormat(w, deps, p, f); err != nil {
			return err
		}
		writeProvenance(w, f, prov)
		return postFn()
	}

	// Multi-format to files. Also render markdown to post buffer if posting.
	if pc != nil {
		if err := renderFormat(&pc.buf, deps, p, format.Markdown); err != nil {
			return err
		}
		writeProvenance(&pc.buf, format.Markdown, prov)
	}

	slug := commandSlug(cmd)
	var written []string
	for _, f := range deps.Output.Results {
		name := slug + "." + formatExt(f)
		path := filepath.Join(deps.Output.WriteTo, name)

		if err := writeFileAtomic(path, func(w *os.File) error {
			if err := renderFormat(w, deps, p, f); err != nil {
				return err
			}
			writeProvenance(w, f, prov)
			return nil
		}); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		written = append(written, name)
	}
	log.Debug("artifacts written to %s (%s)", deps.Output.WriteTo, strings.Join(written, ", "))
	return postFn()
}

// renderFormat renders a Pipeline in the given format to w.
// For HTML, renders as markdown first then converts via goldmark.
func renderFormat(w interface{ Write([]byte) (int, error) }, deps *Deps, p pipeline.Pipeline, f format.Format) error {
	if f == format.HTML {
		// Render markdown to buffer, convert to HTML, wrap in shell.
		var mdBuf bytes.Buffer
		rc := format.RenderContext{
			Writer: &mdBuf,
			Format: format.Markdown,
			IsTTY:  false,
			Width:  deps.TermWidth,
			Owner:  deps.Owner,
			Repo:   deps.Repo,
		}
		if err := p.Render(rc); err != nil {
			return err
		}
		slug := deps.Owner + "/" + deps.Repo
		return format.WriteReportHTML(w, mdBuf.String(), "Velocity: "+slug)
	}

	rc := format.RenderContext{
		Writer: w,
		Format: f,
		IsTTY:  deps.IsTTY,
		Width:  deps.TermWidth,
		Owner:  deps.Owner,
		Repo:   deps.Repo,
	}
	return p.Render(rc)
}

// provenanceRenderer is an optional interface for pipelines that render
// their own provenance (e.g., velocity with its multi-line config block).
// When a pipeline implements this, renderPipeline skips the generic footer.
type provenanceRenderer interface {
	HasProvenance() bool
}

// writeProvenance writes provenance metadata after the main output.
// For pretty: single-line footer. For markdown: <details> block.
// For JSON: caller must include provenance in its JSON struct.
func writeProvenance(w io.Writer, f format.Format, prov model.Provenance) {
	if prov.Command == "" {
		return
	}
	switch f {
	case format.Pretty:
		prov.WriteFooter(w)
	case format.Markdown:
		format.RenderProvenanceMarkdown(w, prov, "")
	case format.JSON:
		// JSON provenance is embedded in the JSON struct by the caller,
		// not appended separately. Write nothing here.
	}
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
	case format.HTML:
		return "html"
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
