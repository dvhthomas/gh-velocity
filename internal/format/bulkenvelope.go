package format

// BulkEnvelope holds the shared JSON fields for bulk metric output.
// Per-command JSON output types embed this to avoid duplicating the
// envelope fields (repository, window, sort, stats, etc.).
//
// Example usage in a metric's render.go:
//
//	type jsonBulkOutput struct {
//	    format.BulkEnvelope
//	    Strategy string           `json:"strategy,omitempty"` // metric-specific
//	    Items    []jsonBulkItem   `json:"items"`
//	}
type BulkEnvelope struct {
	Repository string        `json:"repository"`
	Window     JSONWindow    `json:"window"`
	SearchURL  string        `json:"search_url"`
	Sort       JSONSort      `json:"sort"`
	Stats      JSONStats     `json:"stats"`
	Capped     bool          `json:"capped,omitempty"`
	Warnings   []string      `json:"warnings,omitempty"`
	Insights   []JSONInsight `json:"insights,omitempty"`
}
