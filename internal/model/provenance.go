package model

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// provenanceJSON is the JSON wire format for Provenance.
type provenanceJSON struct {
	Command string            `json:"command"`
	Config  map[string]string `json:"config,omitempty"`
}

// WriteJSON writes provenance as a JSON object to w.
// Returns nil without writing if provenance is empty.
func (p Provenance) WriteJSON(w io.Writer) error {
	if p.Command == "" {
		return nil
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(provenanceJSON{
		Command: p.Command,
		Config:  p.Config,
	})
}

// MarshalJSON implements json.Marshaler.
func (p Provenance) MarshalJSON() ([]byte, error) {
	if p.Command == "" {
		return []byte("null"), nil
	}
	return json.Marshal(provenanceJSON{
		Command: p.Command,
		Config:  p.Config,
	})
}

// WritePretty writes provenance as indented text to w.
func (p Provenance) WritePretty(w io.Writer) {
	if p.Command == "" {
		return
	}
	fmt.Fprintf(w, "\n  Command: %s\n", p.Command)
	if len(p.Config) > 0 {
		keys := sortedKeys(p.Config)
		for _, k := range keys {
			fmt.Fprintf(w, "    %s: %s\n", k, p.Config[k])
		}
	}
}

// WriteMarkdown writes provenance as markdown content suitable for
// inclusion inside a <details> block.
func (p Provenance) WriteMarkdown(w io.Writer) {
	if p.Command != "" {
		fmt.Fprintf(w, "**Command**: `%s`\n\n", p.Command)
	}
	if len(p.Config) > 0 {
		fmt.Fprintln(w, "**Config**:")
		keys := sortedKeys(p.Config)
		for _, k := range keys {
			fmt.Fprintf(w, "- %s: `%s`\n", k, p.Config[k])
		}
	}
}

// MarkdownString returns provenance as a markdown string, suitable for
// embedding in templates.
func (p Provenance) MarkdownString() string {
	if p.Command == "" && len(p.Config) == 0 {
		return ""
	}
	var s string
	if p.Command != "" {
		s += fmt.Sprintf("**Command**: `%s`\n\n", p.Command)
	}
	if len(p.Config) > 0 {
		s += "**Config**:\n"
		keys := sortedKeys(p.Config)
		for _, k := range keys {
			s += fmt.Sprintf("- %s: `%s`\n", k, p.Config[k])
		}
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
