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

// WriteInsightsPretty writes insights as a bulleted list.
func WriteInsightsPretty(w io.Writer, insights []Insight) {
	if len(insights) == 0 {
		return
	}
	fmt.Fprintln(w)
	for _, ins := range insights {
		fmt.Fprintf(w, "  → %s\n", ins.Message)
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
