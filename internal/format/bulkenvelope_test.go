package format

import (
	"encoding/json"
	"testing"
)

func TestBulkEnvelope_EmbeddedMarshal(t *testing.T) {
	// Simulate how cycletime embeds the envelope with an extra Strategy field.
	type withStrategy struct {
		BulkEnvelope
		Strategy string `json:"strategy"`
		Items    []struct {
			Number int `json:"number"`
		} `json:"items"`
	}

	out := withStrategy{
		BulkEnvelope: BulkEnvelope{
			Repository: "org/repo",
			Window:     JSONWindow{Since: "2026-01-01T00:00:00Z", Until: "2026-02-01T00:00:00Z"},
			SearchURL:  "https://github.com/search?q=test",
			Sort:       JSONSort{Field: "cycle_time", Direction: "desc"},
			Stats:      JSONStats{Count: 5},
			Warnings:   []string{"test"},
		},
		Strategy: "issue",
		Items:    []struct{ Number int `json:"number"` }{{Number: 1}},
	}

	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Verify all fields are at the top level (not nested under an "envelope" key).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	for _, key := range []string{"repository", "window", "search_url", "sort", "stats", "warnings", "strategy", "items"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing top-level key %q in marshaled JSON", key)
		}
	}

	// Verify there is no "envelope" or "BulkEnvelope" nested key.
	for _, bad := range []string{"envelope", "BulkEnvelope", "bulk_envelope"} {
		if _, ok := raw[bad]; ok {
			t.Errorf("unexpected nested key %q — embedded fields should be promoted", bad)
		}
	}
}

func TestBulkEnvelope_WithoutStrategy(t *testing.T) {
	// Simulate how leadtime embeds the envelope WITHOUT a Strategy field.
	type withoutStrategy struct {
		BulkEnvelope
		Items []struct {
			Number int `json:"number"`
		} `json:"items"`
	}

	out := withoutStrategy{
		BulkEnvelope: BulkEnvelope{
			Repository: "org/repo",
			Window:     JSONWindow{Since: "2026-01-01T00:00:00Z", Until: "2026-02-01T00:00:00Z"},
			Stats:      JSONStats{Count: 3},
		},
		Items: []struct{ Number int `json:"number"` }{{Number: 1}},
	}

	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Should NOT have a strategy field.
	if _, ok := raw["strategy"]; ok {
		t.Error("leadtime-style output should not have 'strategy' field")
	}

	// Omitempty fields should be absent when zero.
	if _, ok := raw["warnings"]; ok {
		t.Error("empty warnings should be omitted")
	}
	if _, ok := raw["capped"]; ok {
		t.Error("false capped should be omitted")
	}
}
