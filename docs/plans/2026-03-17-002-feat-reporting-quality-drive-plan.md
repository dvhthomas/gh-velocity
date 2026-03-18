---
title: "feat: Reporting quality drive — noise detection, doc links, missing sections"
type: feat
status: active
date: 2026-03-17
origin: docs/brainstorms/2026-03-17-reporting-quality-drive-brainstorm.md
---

# Reporting Quality Drive

## Overview

The daily showcase run ([Discussion #96](https://github.com/dvhthomas/gh-velocity/discussions/96)) revealed 14 UX issues that undermine the credibility and usefulness of report output. cli/cli shows a 1-minute median lead time because ~50 spam issues close in seconds, producing absurd "76798x longer than the median" messages. Stat names like "CV 2.7" are opaque without links. Quality has no detail section. This plan addresses all of these in three phases.

## Problem Statement

1. **Noise** — Spam/invalid issues dominate stats for repos like cli/cli. The median becomes meaningless, outlier multipliers explode, and "73 hotfixes" is really "73 spam issues."
2. **Discoverability** — Stat names (CV, P90), issue numbers (#12730), and metric names (Lead Time) in output are plain text. Readers can't click to learn what they mean or navigate to the referenced issue.
3. **Missing sections** — Quality is a single summary table row with no detail. Throughput detail is bare (just counts). Cycle time detail shows 89 N/A rows alongside 23 real data rows. WIP silently vanishes when unconfigured.

## Proposed Solution

Three phases of rendering/insight-layer changes. No new API calls. No data model changes beyond adding `URL` to `ItemRef`.

### Cross-cutting design decision: format-aware insight messages

Insight messages are generated once during `ProcessData()`, before output format is known. Changes (b) and (c) require markdown links in messages. Resolution:

**Always generate markdown in insight messages.** Add `StripMarkdownLinks(s string) string` to `internal/format/markdown.go` alongside the existing `StripMarkdownBold()`. Pretty-text rendering paths already call `StripMarkdownBold()` — extend those call sites to also strip links. JSON consumers receive messages with markdown link syntax (this is acceptable — same contract as the existing bold markers).

This follows the established pattern: `FormatStatsDetail()` already generates `**bold**` markers that get stripped for pretty output.

---

## Phase 1: Data Quality (P1 — highest impact)

### 1a. Noise detection insight

When >= 3 items have duration < 60 seconds, surface an insight suggesting scope exclusion.

**File:** `internal/metrics/report_insights.go`

Add constants:
```go
NoiseMinCount      = 3
NoiseMaxDuration   = 60 * time.Second
```

Add to `GenerateStatsInsights()`, before outlier detection:
```go
// Noise detection — many sub-minute items suggest spam/automation.
var noiseCount int
for _, item := range items {
    if item.Duration > 0 && item.Duration < NoiseMaxDuration {
        noiseCount++
    }
}
if noiseCount >= NoiseMinCount {
    insights = append(insights, model.Insight{
        Type:    "noise_detection",
        Message: fmt.Sprintf("%d issues closed in under 60 seconds — consider narrowing scope to exclude noise (e.g., `scope: '-label:invalid'`).", noiseCount),
    })
}
```

**Tests first** (`report_insights_test.go`):
- "noise detection fires when >= 3 sub-60s items" — 5 items at 10s, 20s, 30s + 2 at 5d → wantCount includes noise insight
- "noise detection silent when < 3 sub-60s items" — 2 items at 10s + 5 at 5d → no noise insight
- "noise detection silent when no items" → no insight

### 1b. Cap outlier multiplier at 100x + low-median diagnostic

**File:** `internal/metrics/report_insights.go` — outlier detection block

Cap the multiplier:
```go
if multiple > 100 {
    insights = append(insights, model.Insight{
        Type:    "outlier_detection",
        Message: fmt.Sprintf("%d items took 100x+ longer than the median (%s).",
            stats.OutlierCount, fmtDur(*stats.Median)),
    })
} else {
    // existing message with exact multiple
}
```

Add low-median diagnostic (new constant `LowMedianThreshold = time.Hour`, `LowMedianMinCount = 10`):
```go
if stats.Median != nil && *stats.Median < LowMedianThreshold && stats.Count > LowMedianMinCount {
    insights = append(insights, model.Insight{
        Type:    "low_median",
        Message: fmt.Sprintf("Median is %s — likely distorted by noise issues closed in seconds.",
            fmtDur(*stats.Median)),
    })
}
```

**Interaction rule:** If `noise_detection` fires, suppress `low_median` (they diagnose the same root cause). Implement by checking whether a noise insight was already appended.

**Tests first:**
- "outlier multiplier capped at 100x" — median=1m, cutoff=200d → message says "100x+"
- "outlier multiplier shows exact value when <= 100" — median=5d, cutoff=30d → "6x"
- "low-median fires when median < 1h and count > 10" — median=30s, count=50 → fires
- "low-median suppressed when noise already fired" — same data with noise items → only noise fires

### 1c. Filter N/A rows from cycle time detail

**Files:** `internal/pipeline/cycletime/render.go` (WriteBulkMarkdown + WriteBulkPretty)

Filter items before building rows:
```go
var withData []BulkItem
for _, item := range sorted {
    if item.Metric.Duration != nil {
        withData = append(withData, item)
    }
}
```

Use `withData` for building the row slice. Update `<summary>` count:
- When filtered: `Details (15 of 30 items)` — shows both measured and total
- When no filtering needed: `Details (15 items)` — same as today

**Template change** (`cycletime-bulk.md.tmpl`): Replace `{{len .Items}}` with `{{.DetailCount}}` in summary, add `DetailCount` and `TotalCount` to `bulkTemplateData`.

Apply the same filter to `WriteBulkPretty` for consistency across formats (see brainstorm: SpecFlow Q4).

**Tests first:**
- Unit test: build BulkItems with mix of nil/non-nil durations, verify filtered count
- Integration: render markdown, verify N/A rows absent, summary shows "X of Y items"

### 1d. Min-duration threshold for fastest/slowest

**File:** `internal/metrics/report_insights.go` — `findExtremes()`

Add constant `FastestSlotMinDuration = time.Minute`.

```go
func findExtremes(items []ItemRef) (*ItemRef, *ItemRef) {
    // Filter out sub-minute items to avoid picking spam as "fastest"
    var eligible []ItemRef
    for _, item := range items {
        if item.Duration >= FastestSlotMinDuration {
            eligible = append(eligible, item)
        }
    }
    if len(eligible) == 0 {
        return nil, nil
    }
    // existing min/max logic on eligible
}
```

**Tests first:**
- "fastest/slowest skips sub-minute items" — items at 10s, 30s, 2d, 5d → fastest=2d
- "fastest/slowest returns nil when all sub-minute" → no insight
- "fastest/slowest works normally when no sub-minute items" → unchanged behavior

---

## Phase 2: Links & Discoverability (P2 — high polish)

### 2a. DocSiteURL constant

**File:** `internal/format/links.go`

```go
// DocSiteURL is the base URL for the gh-velocity documentation site.
const DocSiteURL = "https://dvhthomas.github.io/gh-velocity"
```

Add helper:
```go
// DocLink returns a markdown link to a documentation page.
// anchor is the path after the base URL, e.g., "/concepts/statistics/#...".
func DocLink(text, anchor string) string {
    return fmt.Sprintf("[%s](%s%s)", text, DocSiteURL, anchor)
}
```

### 2b. StripMarkdownLinks helper

**File:** `internal/format/markdown.go`

```go
// StripMarkdownLinks removes [text](url) links, keeping only the text.
func StripMarkdownLinks(s string) string {
    // Replace [text](url) with text
    re := regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
    return re.ReplaceAllString(s, "$1")
}
```

Compile the regex once at package level as `var markdownLinkRe = regexp.MustCompile(...)`.

**Tests first:**
- `StripMarkdownLinks("[CV](url) is 2.7")` → `"CV is 2.7"`
- `StripMarkdownLinks("[#123](url) took 5d")` → `"#123 took 5d"`
- `StripMarkdownLinks("no links here")` → `"no links here"`
- `StripMarkdownLinks("[a](url1) and [b](url2)")` → `"a and b"`

### 2c. Stat name links in insight messages

**Important:** The `metrics` package is pure computation — it must NOT import `format`. Doc links in insight messages are added at the rendering layer, not in the insight generator.

**Approach:** Insight messages stay as plain text (e.g., `"CV 2.7"`). The `format` package wraps known terms with doc links when rendering for markdown. Add a helper:

**File:** `internal/format/links.go`

```go
// LinkStatTerms wraps known statistical terms in insight messages with doc links.
// Only applies to markdown output — pretty callers should not call this.
func LinkStatTerms(msg string) string {
    // Replace standalone "CV " with linked version (space after to avoid matching "CVS" etc.)
    msg = strings.Replace(msg, "(CV ", "("+DocLink("CV", "/concepts/statistics/#coefficient-of-variation-cv")+" ", 1)
    return msg
}
```

Apply `LinkStatTerms` when rendering insight messages in markdown format:
- `internal/format/report.go` — in `buildInsightGroups`, wrap messages
- `internal/pipeline/leadtime/render.go` — when building `insightMsgs` slice
- `internal/pipeline/cycletime/render.go` — same

**In FormatStatsDetail** (`internal/format/report.go`):
```go
// Before: fmt.Sprintf("**Predictability:** %s (CV %.1f)", label, *cv)
// After:
fmt.Sprintf("**Predictability:** %s (%s %.1f)", label, DocLink("CV", "/concepts/statistics/#coefficient-of-variation-cv"), *cv)
```

Pretty-text rendering already strips bold. Add `StripMarkdownLinks` alongside `StripMarkdownBold` in the pretty rendering paths:
- `internal/pipeline/leadtime/render.go` line ~188: `format.StripMarkdownLinks(format.StripMarkdownBold(line))`
- `internal/pipeline/cycletime/render.go` line ~258: same

### 2d. URL on ItemRef + issue links in insights

**File:** `internal/metrics/report_insights.go`

Add field:
```go
type ItemRef struct {
    Number   int
    Title    string
    Duration time.Duration
    Category string
    URL      string // GitHub issue/PR URL
}
```

Update `fastest_slowest` insight to use markdown links:
```go
// Before: fmt.Sprintf("Fastest: #%d %s (%s)", ...)
// After:
fmtRef := func(r *ItemRef) string {
    if r.URL != "" {
        return fmt.Sprintf("[#%d](%s) %s (%s)", r.Number, r.URL, truncate(r.Title, 40), fmtDur(r.Duration))
    }
    return fmt.Sprintf("#%d %s (%s)", r.Number, truncate(r.Title, 40), fmtDur(r.Duration))
}
```

**Propagation sites** (add `URL: item.Issue.URL` or equivalent):
- `internal/pipeline/leadtime/leadtime.go` ~line 139
- `internal/pipeline/cycletime/cycletime.go` ~line 259
- `cmd/report.go` ~line 559

**Tests first:**
- Verify ItemRef with URL produces `[#N](url)` in fastest/slowest message
- Verify ItemRef without URL produces `#N` (no broken link)

### 2e. Metric name links in report table

**File:** `internal/format/templates/report.md.tmpl`

Add a `docLink` function to the shared template funcMap in `internal/format/templates.go`:

```go
"docLink": func(text, anchor string) string {
    return DocLink(text, anchor)
},
```

Then use it in the template (single source of truth for the base URL):
```
{{- if .LeadTime}}
| {{docLink "Lead Time" "/reference/metrics/lead-time/"}} | {{.LeadTime}} |
{{- end}}
{{- if .CycleTime}}
| {{docLink "Cycle Time" "/reference/metrics/cycle-time/"}} | {{.CycleTime}} |
{{- end}}
{{- if .Throughput}}
| {{docLink "Throughput" "/reference/metrics/throughput/"}} | {{.Throughput}} |
{{- end}}
{{- if .Velocity}}
| {{docLink "Velocity" "/reference/metrics/velocity/"}} | {{.Velocity}} |
{{- end}}
{{- if .WIP}}
| WIP | {{.WIP}} |
{{- end}}
{{- if .Quality}}
| {{docLink "Quality" "/reference/metrics/quality/"}} | {{.Quality}} |
{{- end}}
```

WIP is not linked because it doesn't have a reference page yet. Add link when WIP is implemented.

Pretty-text output (`WriteReportPretty` in `report.go`) uses hardcoded strings — no change needed (links are markdown-only per brainstorm decision #6).

---

## Phase 3: Missing Output Sections (P2-P3 — completeness)

### 3a. Quality detail section in report

This is the biggest single change. Currently Quality is a single line: `29 bugs / 112 issues (26% defect rate)`. It should have a full detail section showing:

- Category breakdown table (category, count, percentage)
- Per-item table (issue, title, category, lead time)
- Hotfix count (items resolved within hotfix window)

**Data flow change:**

`computeQualityWithInsights()` in `cmd/report.go` currently discards the per-item classification. Expand its return to include the classified items:

```go
// Before:
func computeQualityWithInsights(...) (*model.StatsQuality, []model.Insight)

// After:
type qualityDetail struct {
    Quality     model.StatsQuality
    Insights    []model.Insight
    Items       []qualityItemRow  // per-item classification for detail table
    Categories  []qualityCategoryRow  // category breakdown
    HotfixCount int
}

func computeQualityWithInsights(...) qualityDetail
```

The `qualityItemRow` and `qualityCategoryRow` types live in `cmd/report.go` (local to the report command, not exported).

**Rendering:**

Create `internal/pipeline/quality/render.go` and `templates/quality-bulk.md.tmpl` following the leadtime/cycletime pattern:

Template structure:
```markdown
## Quality: {{.Repository}} ({{date .Since}} – {{date .Until}} UTC)
{{- if .Insights}}

**Insights:**
{{- range .Insights}}
- {{.}}
{{- end}}
{{- end}}

**Category Breakdown:**

| Category | Count | Percentage |
| --- | ---: | ---: |
{{- range .Categories}}
| {{.Name}} | {{.Count}} | {{.Pct}}% |
{{- end}}

Defect rate: {{.DefectRate}}% ({{.BugCount}} bugs / {{.TotalIssues}} issues)
{{if .Items}}
<details>
<summary>Details ({{len .Items}} issues)</summary>

| Issue | Title | Category | Lead Time |
| ---: | --- | --- | --- |
{{- range .Items}}
| {{.Link}} | {{.Title}} | {{.Category}} | {{.LeadTime}} |
{{- end}}

</details>
{{- end}}
```

Wire into `cmd/report.go` detail section block alongside the existing Lead Time / Cycle Time / Throughput / Velocity detail sections using the `writeDetail` helper.

**Note on releases/tags:** The report command operates on a time window of closed issues — it does not fetch release data. Release-level quality data (per-release composition, cadence, hotfix detection) lives in the standalone `quality release` command. Adding release data to the report would require additional API calls and is out of scope for this quality drive. Future enhancement: a "Recent Releases" detail section in the report that calls the release pipeline.

**Tests first:**
- `report_artifacts_test.go`: verify quality detail section exists in report.md, verify quality artifact file created
- Unit test for quality detail rendering: category breakdown, per-item table, hotfix count

### 3b. Defect rate >60% insight

**File:** `internal/metrics/report_insights.go` — `GenerateQualityInsights()`

Add constant `DefectRateSuspicious = 0.60`.

When defect rate > 60%, fire a `defect_rate_review` insight **instead of** the existing `defect_rate_high` insight (see brainstorm: SpecFlow Q3 — replace, don't supplement, to avoid contradictory messages):

```go
switch {
case quality.DefectRate > DefectRateSuspicious:
    insights = append(insights, model.Insight{
        Type:    "defect_rate_review",
        Message: fmt.Sprintf("%.0f%% defect rate — may reflect issue template naming rather than actual bugs. Review category matchers.", quality.DefectRate*100),
    })
case quality.DefectRate > DefectRateHigh:
    // existing message
}
```

**Tests first:**
- "defect rate >60% fires review insight" — 80% defect rate → `defect_rate_review`, NOT `defect_rate_high`
- "defect rate 20-60% fires high insight" — 40% → `defect_rate_high` only
- "defect rate <=20% silent" → no defect insight

### 3c. Extreme median insight

**File:** `internal/metrics/report_insights.go` — `GenerateStatsInsights()`

Add constant `ExtremeMedianThreshold = 365 * 24 * time.Hour`.

```go
if stats.Median != nil && *stats.Median > ExtremeMedianThreshold {
    insights = append(insights, model.Insight{
        Type:    "extreme_median",
        Message: fmt.Sprintf("Median is %s — likely includes backlog cleanup alongside recent work.", fmtDur(*stats.Median)),
    })
}
```

**Tests first:**
- "extreme median fires when > 365 days" — median=500d → fires
- "extreme median silent when <= 365 days" — median=100d → silent

### 3d. Richer Throughput detail

**File:** `internal/pipeline/throughput/render.go` and template

Add category breakdown table to the throughput detail section. The data is already available in the report path — `computeQualityWithInsights()` classifies all items by category. Thread the `categoryDist map[string]int` to the throughput renderer.

Update the throughput template to include:
```markdown
{{- if .Categories}}

**Category Breakdown:**

| Category | Count | Percentage |
| --- | ---: | ---: |
{{- range .Categories}}
| {{.Name}} | {{.Count}} | {{.Pct}}% |
{{- end}}
{{- end}}
```

Scoped to the report path only — the standalone `flow throughput` command does not have access to classification (see brainstorm: SpecFlow Q5).

Add `Categories []categoryRow` to the throughput template data struct (existing `throughputTemplateData` or a new report-specific wrapper).

### 3e. WIP "not configured" row

**File:** `internal/format/report.go` (WriteReportPretty) and `templates/report.md.tmpl`

When `r.WIPCount == nil` and at least one other section has data, show a "not configured" row instead of silently omitting WIP:

Pretty:
```go
fmt.Fprintf(w, "  WIP:         not configured\n")
```

Markdown template:
```
{{- if .WIP}}
| WIP | {{.WIP}} |
{{- else if .HasData}}
| WIP | not configured |
{{- end}}
```

Add `HasData bool` to `reportTemplateData` — set to true when any section has data.

JSON: omit the WIP field when nil (current behavior). JSON consumers should treat absent fields as "not available" (see brainstorm: SpecFlow Q3 — keep JSON sparse).

---

## Files to Change (Complete)

| File | Phase | What |
|------|-------|------|
| `internal/metrics/report_insights.go` | 1, 2, 3 | Noise insight, multiplier cap, low-median, findExtremes filter, defect review, extreme median, ItemRef.URL, stat links |
| `internal/metrics/report_insights_test.go` | 1, 2, 3 | Tests for all new/modified insight rules |
| `internal/format/links.go` | 2 | `DocSiteURL` constant, `DocLink()` helper |
| `internal/format/markdown.go` | 2 | `StripMarkdownLinks()` |
| `internal/format/report.go` | 2, 3 | Stat name links in FormatStatsDetail, WIP "not configured" |
| `internal/format/templates/report.md.tmpl` | 2, 3 | Metric name links, WIP row, HasData |
| `internal/format/templates.go` | 2, 3 | `docLink` in funcMap, `HasData` field on reportTemplateData |
| `internal/pipeline/cycletime/render.go` | 1 | Filter N/A rows, update summary count |
| `internal/pipeline/cycletime/templates/cycletime-bulk.md.tmpl` | 1 | DetailCount/TotalCount in summary |
| `internal/pipeline/leadtime/render.go` | 2 | StripMarkdownLinks in pretty path |
| `internal/pipeline/cycletime/cycletime.go` | 2 | ItemRef.URL propagation |
| `internal/pipeline/leadtime/leadtime.go` | 2 | ItemRef.URL propagation |
| `internal/pipeline/quality/render.go` | 3 | **New file** — quality detail renderer |
| `internal/pipeline/quality/templates/quality-bulk.md.tmpl` | 3 | **New file** — quality detail template |
| `internal/pipeline/throughput/render.go` | 3 | Category breakdown table |
| `internal/pipeline/throughput/templates/throughput.md.tmpl` | 3 | Category breakdown template section |
| `cmd/report.go` | 2, 3 | ItemRef.URL propagation, quality detail wiring, throughput categories, WIP row |
| `cmd/report_artifacts_test.go` | 3 | Quality detail assertions |

## Acceptance Criteria

### Phase 1: Data Quality
- [ ] cli/cli report shows noise detection insight when many sub-60s items
- [ ] Outlier multiplier never exceeds "100x+" in any message
- [ ] Low-median diagnostic fires when median < 1h and n > 10 (suppressed by noise insight)
- [ ] Cycle time detail table omits N/A rows; summary shows "X of Y items"
- [ ] Fastest/slowest insight never picks a sub-minute item

### Phase 2: Links
- [ ] CV in insight messages links to statistics docs page (markdown only)
- [ ] Issue numbers in fastest/slowest insight link to GitHub (markdown only)
- [ ] Metric names in report summary table link to reference pages (markdown only)
- [ ] Pretty-text output shows no raw URLs or markdown link syntax
- [ ] `DocSiteURL` defined in one location only

### Phase 3: Missing Sections
- [ ] Quality detail section shows category breakdown table + per-item table + hotfix count
- [ ] Defect rate > 60% shows "review category matchers" (replaces generic "above threshold")
- [ ] Median > 365 days shows "likely includes backlog cleanup"
- [ ] Throughput detail includes category breakdown table (report path only)
- [ ] WIP shows "not configured" when nil (markdown + pretty; JSON omits)

### All Phases
- [ ] `go test ./... -count=1` passes after each phase
- [ ] Showcase workflow run produces improved output for cli/cli
- [ ] Tests written BEFORE implementation for each change

## Verification

After each phase:
1. `go test ./... -count=1` — all pass
2. Build binary, run `./gh-velocity report --since 30d -R cli/cli --format markdown` — verify noise detection, multiplier cap, filtered N/A rows
3. Run same command with `--format json` — verify JSON output is clean
4. Run showcase workflow, review discussion output for all 6 repos
5. Verify doc links resolve to actual pages (check `site/content/` paths exist)

## Sources

- **Origin brainstorm:** [docs/brainstorms/2026-03-17-reporting-quality-drive-brainstorm.md](docs/brainstorms/2026-03-17-reporting-quality-drive-brainstorm.md) — Key decisions: insight-only noise detection, hardcoded DocSiteURL, URL on ItemRef, keep insight duplication, cap multiplier at 100x, format-aware links (markdown only)
- **Showcase output:** [Discussion #96](https://github.com/dvhthomas/gh-velocity/discussions/96) — the data that exposed all 14 issues
- **Related issue:** [#81](https://github.com/dvhthomas/gh-velocity/issues/81) — noise/spam exclusion
- **SpecFlow analysis:** Resolved Q1 (always-markdown + strip), Q2 (expand computeQualityWithInsights return), Q3 (>60% replaces >20%), Q4 (filter both formats), Q5 (throughput categories report-only), Q7 ("X of Y items")
