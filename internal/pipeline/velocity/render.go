package velocity

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"text/template"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

//go:embed templates/*.md.tmpl
var templateFS embed.FS

var markdownTmpl = template.Must(
	template.New("velocity.md.tmpl").Funcs(velocityFuncMap()).ParseFS(templateFS, "templates/velocity.md.tmpl"),
)

func velocityFuncMap() template.FuncMap {
	m := format.TemplateFuncMap()
	m["notAssessedHint"] = notAssessedHint
	return m
}

// --- JSON ---

type jsonOutput struct {
	Repository   string           `json:"repository"`
	Unit         string           `json:"unit"`
	EffortUnit   string           `json:"effort_unit"`
	Provenance   model.Provenance `json:"provenance,omitempty"`
	EffortDetail jsonEffort       `json:"effort"`
	Current      *jsonIteration   `json:"current,omitempty"`
	History      []jsonIteration  `json:"history,omitempty"`
	Summary      jsonSummary      `json:"summary"`
}


type jsonEffort struct {
	Strategy     string             `json:"strategy"`
	Matchers     []jsonEffortMatch  `json:"matchers,omitempty"`
	NumericField string             `json:"numeric_field,omitempty"`
}

type jsonEffortMatch struct {
	Query string  `json:"query"`
	Value float64 `json:"value"`
}

type jsonIteration struct {
	Name          string  `json:"name"`
	Start         string  `json:"start"`
	End           string  `json:"end"`
	Velocity      float64 `json:"velocity"`
	Committed     float64 `json:"committed"`
	CompletionPct float64 `json:"completion_pct"`
	ItemsDone     int     `json:"items_done"`
	ItemsTotal    int     `json:"items_total"`
	NotAssessed   int     `json:"not_assessed"`
	Trend         string  `json:"trend"`
}

type jsonSummary struct {
	AvgVelocity   float64 `json:"avg_velocity"`
	AvgCompletion float64 `json:"avg_completion_pct"`
	StdDev        float64 `json:"std_dev"`
}

func toJSONIteration(iv model.IterationVelocity) jsonIteration {
	return jsonIteration{
		Name:          iv.Name,
		Start:         iv.Start.UTC().Format(time.DateOnly),
		End:           iv.End.UTC().Format(time.DateOnly),
		Velocity:      iv.Velocity,
		Committed:     iv.Committed,
		CompletionPct: iv.CompletionPct,
		ItemsDone:     iv.ItemsDone,
		ItemsTotal:    iv.ItemsTotal,
		NotAssessed:   iv.NotAssessed,
		Trend:         iv.Trend,
	}
}

// WriteJSON writes velocity as JSON.
func WriteJSON(w io.Writer, r model.VelocityResult) error {
	je := jsonEffort{Strategy: r.EffortDetail.Strategy}
	for _, m := range r.EffortDetail.Matchers {
		je.Matchers = append(je.Matchers, jsonEffortMatch{Query: m.Query, Value: m.Value})
	}
	je.NumericField = r.EffortDetail.NumericField

	out := jsonOutput{
		Repository:   r.Repository,
		Unit:         r.Unit,
		EffortUnit:   r.EffortUnit,
		EffortDetail: je,
		Summary: jsonSummary{
			AvgVelocity:   r.AvgVelocity,
			AvgCompletion: r.AvgCompletion,
			StdDev:        r.StdDev,
		},
	}
	if r.Current != nil {
		ji := toJSONIteration(*r.Current)
		out.Current = &ji
	}
	for _, h := range r.History {
		out.History = append(out.History, toJSONIteration(h))
	}
	out.Provenance = r.Provenance
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- Pretty ---

// WritePretty writes velocity as formatted text.
func WritePretty(w io.Writer, r model.VelocityResult, verbose bool) error {
	fmt.Fprintf(w, "Velocity: %s (%s)\n\n", r.Repository, r.Unit)

	if r.Current != nil {
		c := r.Current
		fmt.Fprintf(w, "  Current: %s (%s – %s)\n",
			c.Name, c.Start.Format(time.DateOnly), c.End.Format(time.DateOnly))
		fmt.Fprintf(w, "    Velocity:       %.1f %s\n", c.Velocity, r.EffortUnit)
		fmt.Fprintf(w, "    Committed:      %.1f %s\n", c.Committed, r.EffortUnit)
		fmt.Fprintf(w, "    Completion:     %.0f%%\n", c.CompletionPct)
		fmt.Fprintf(w, "    Items:          %d / %d done\n", c.ItemsDone, c.ItemsTotal)
		if c.NotAssessed > 0 {
			fmt.Fprintf(w, "    Not assessed:   %d %s\n", c.NotAssessed, notAssessedHint(r.EffortDetail.Strategy))
			if verbose && len(c.NotAssessedItems) > 0 {
				fmt.Fprintf(w, "      Items: %s\n", formatItemNumbers(c.NotAssessedItems))
			}
		}
		fmt.Fprintln(w)
	}

	if len(r.History) > 0 {
		fmt.Fprintf(w, "  History:\n")
		// Header.
		fmt.Fprintf(w, "    %-20s %8s %8s %8s %6s %s\n",
			"Iteration", "Velocity", "Commit", "Done%", "Items", "Trend")
		fmt.Fprintf(w, "    %-20s %8s %8s %8s %6s %s\n",
			"─────────", "────────", "──────", "─────", "─────", "─────")
		for _, h := range r.History {
			fmt.Fprintf(w, "    %-20s %8.1f %8.1f %7.0f%% %3d/%-3d %s\n",
				truncate(h.Name, 20),
				h.Velocity, h.Committed, h.CompletionPct,
				h.ItemsDone, h.ItemsTotal,
				h.Trend)
		}
		fmt.Fprintln(w)

		fmt.Fprintf(w, "  Summary:\n")
		fmt.Fprintf(w, "    Avg velocity:   %.1f %s\n", r.AvgVelocity, r.EffortUnit)
		fmt.Fprintf(w, "    Avg completion: %.0f%%\n", r.AvgCompletion)
		if r.StdDev > 0 {
			fmt.Fprintf(w, "    Std dev:        %.1f\n", r.StdDev)
		}
	}

	writeEffortDetailPretty(w, r.EffortDetail)
	r.Provenance.WritePretty(w)

	return nil
}

func writeEffortDetailPretty(w io.Writer, d model.EffortDetail) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Effort strategy:")
	switch d.Strategy {
	case "count":
		fmt.Fprintln(w, "    count — every item = 1")
	case "attribute":
		fmt.Fprintln(w, "    attribute — label/type matchers (first match wins):")
		for _, m := range d.Matchers {
			fmt.Fprintf(w, "      %-30s → %.0f\n", m.Query, m.Value)
		}
	case "numeric":
		fmt.Fprintf(w, "    numeric — project field %q\n", d.NumericField)
	}
}

// --- Markdown ---

type templateData struct {
	Repository     string
	Unit           string
	EffortUnit     string
	EffortStrategy string // for notAssessedHint in current iteration
	Current        *model.IterationVelocity
	History        []model.IterationVelocity
	AvgVel         float64
	AvgComp        float64
	StdDev         float64
}

// WriteMarkdown writes velocity as markdown.
func WriteMarkdown(w io.Writer, r model.VelocityResult) error {
	if err := markdownTmpl.Execute(w, templateData{
		Repository:     r.Repository,
		Unit:           r.Unit,
		EffortUnit:     r.EffortUnit,
		EffortStrategy: r.EffortDetail.Strategy,
		Current:        r.Current,
		History:        r.History,
		AvgVel:         r.AvgVelocity,
		AvgComp:        r.AvgCompletion,
		StdDev:         r.StdDev,
	}); err != nil {
		return err
	}
	return format.RenderProvenanceMarkdown(w, r.Provenance, effortDetailMarkdown(r.EffortDetail))
}

// effortDetailMarkdown returns the effort strategy description as markdown.
func effortDetailMarkdown(d model.EffortDetail) string {
	var s string
	s += fmt.Sprintf("\n**Effort strategy**: %s", d.Strategy)
	switch d.Strategy {
	case "count":
		s += " — every item = 1 (no effort weighting).\n"
	case "attribute":
		s += "\n\nLabel/type matchers (first match wins):\n\n"
		s += "| Matcher | Value |\n|---------|-------|\n"
		for _, m := range d.Matchers {
			s += fmt.Sprintf("| `%s` | %.0f |\n", m.Query, m.Value)
		}
	case "numeric":
		s += fmt.Sprintf(" — project board field: **%s**\n", d.NumericField)
	}
	return s
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// notAssessedHint returns a short explanation of what "not assessed" means
// for the given effort strategy.
func notAssessedHint(strategy string) string {
	switch strategy {
	case "attribute":
		return "(no matching label/type)"
	case "numeric":
		return "(no estimate on board)"
	default:
		return ""
	}
}

func formatItemNumbers(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	s := ""
	for i, n := range nums {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("#%d", n)
	}
	return s
}
