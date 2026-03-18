---
title: "Render-layer linking and insight message quality"
category: "architecture-patterns"
date: "2026-03-17"
tags:
  - output-quality
  - insights
  - formatting
  - links
  - markdown
  - report
  - package-boundaries
module: "internal/format, internal/metrics, internal/pipeline"
severity: "moderate"
---

# Render-Layer Linking and Insight Message Quality

## Problem

The daily showcase run ([Discussion #96](https://github.com/dvhthomas/gh-velocity/discussions/96)) revealed 14 UX issues in report output. The core problems:

1. **Noise distortion**: Spam issues (closed in seconds) dominated stats — cli/cli showed "76798x longer than median" with a 1-minute median
2. **Jargon**: Messages used terms like "heavy right skew", "IQR outlier threshold", and "slow review turnaround" that confused non-technical readers
3. **No links**: Stat names (CV), issue references (#12730), and metric names (Lead Time) were plain text
4. **Data as insights**: Category distributions were restated as insights when they belong in tables
5. **Missing sections**: Quality had no detail section; cycle time showed 89 N/A rows alongside 23 real data rows

## Root Cause

Two architectural gaps:

1. **No render-layer linking mechanism**: The metrics package generated insight messages as plain strings. There was no way to add format-specific links (markdown for GitHub, plain text for terminals) without breaking the package boundary.

2. **No quality gate for insight messages**: There was no defined distinction between "data" (belongs in a table) and "insight" (an observation requiring judgment or comparison). Developers put anything computed into the insight list.

## Solution

### Pattern 1: Format-Aware Messages via Render-Layer Linking

Insight messages always contain markdown (e.g., `[#42 title](url)` links). The render layer adapts per format:

- **Markdown**: messages pass through as-is
- **Pretty**: `StripMarkdownLinks()` and `StripMarkdownBold()` strip formatting
- **JSON**: raw message string with markdown (same contract as bold markers)

The `metrics` package stays pure — it never imports `format`. Instead:

- `ItemRef.URL` lets insight generators embed issue links as markdown
- `LinkStatTerms(msg)` in `internal/format/links.go` wraps known terms (CV, hotfix window, threshold) with doc links at render time
- A `docLink` template function handles metric name links in templates

```go
// internal/format/links.go — render-layer linking
const DocSiteURL = "https://dvhthomas.github.io/gh-velocity"

func DocLink(text, anchor string) string {
    return fmt.Sprintf("[%s](%s%s)", text, DocSiteURL, anchor)
}

func LinkStatTerms(msg string) string {
    msg = strings.Replace(msg, "(CV ", "("+DocLink("CV", "/concepts/statistics/#coefficient-of-variation-cv")+" ", 1)
    msg = strings.Replace(msg, "(hotfix window)", "("+DocLink("hotfix window", "/reference/metrics/quality/#hotfix-detection")+")", 1)
    msg = strings.Replace(msg, "threshold)", DocLink("threshold", "/reference/metrics/quality/#per-release-defect-rate")+")", 1)
    return msg
}
```

Pipeline renderers call `LinkStatTerms` for markdown, never for pretty:

```go
// In leadtime/render.go WriteBulkMarkdown:
for _, ins := range insights {
    insightMsgs = append(insightMsgs, format.LinkStatTerms(ins.Message))
}

// In leadtime/render.go WriteBulkPretty:
fmt.Fprintf(w, "    %s\n", format.StripMarkdownLinks(format.StripMarkdownBold(line)))
```

### Pattern 2: Named Constants + Actual Values in Messages

Every threshold is a named constant. Messages show the actual value and link to docs:

```go
const (
    NoiseMinCount          = 3
    NoiseMaxDuration       = 60 * time.Second
    OutlierMultipleCap     = 100
    DefectRateHigh         = 0.20
    DefectRateSuspicious   = 0.60
    ExtremeMedianThreshold = 365 * 24 * time.Hour
    FastestSlotMinDuration = time.Minute
    LowMedianThreshold     = time.Hour
    LowMedianMinCount      = 10
)
```

Messages include the configured value:
- `"26% of closed issues are bugs (above 20% threshold)."`  — threshold linked to docs
- `"73 items resolved within 72h of creation (hotfix window)."` — hotfix window linked to docs

### Pattern 3: Insight vs Data Distinction

An insight must contain a **judgment or comparison**. If it merely restates what's visible in a table, it's data.

- **Removed**: `"112 items: 43% other, 30% feature, 25% bug."` — this is data for the category table
- **Kept**: `"26% of closed issues are bugs (above 20% threshold)."` — this compares to a threshold
- **Kept**: `"Mean (123d) is much higher than median (1m) — a few slow items are pulling the average up."` — this interprets a pattern

### Pattern 4: N/A Row Filtering

Items without data are filtered at the render layer, not the data layer. Stats computation still receives all items. The summary count shows both values:

```go
data.TotalCount = len(sorted)
for _, item := range sorted {
    if item.Metric.Duration == nil {
        continue
    }
    data.Items = append(data.Items, ...)
}
data.DetailCount = len(data.Items)
```

Template: `Details (23 of 112 issues)` when filtered, `Details (23 issues)` when not.

## Prevention Strategies

### 1. Jargon Creep
- "Explain to a PM" test: would a product manager understand this without Googling?
- Insight messages must not contain statistical method names (IQR, skew, percentile)
- Pattern: `[what happened] + [why it matters]`

### 2. Package Boundary Discipline
- `internal/metrics/` must never import `internal/format/` — enforce structurally
- If a function in metrics returns a string containing markdown or a URL, the boundary is broken
- "Could this run headless?" test: every metrics function should work without an output format

### 3. Broken Doc Links
- Treat doc headings that are link targets as public API — changing them requires searching for references
- `DocSiteURL` defined in one constant; all anchors verified against actual headings
- Consider CI check that validates anchor references

### 4. Data vs Insight
- An insight must compare to something (threshold, baseline, expected range) or identify a pattern
- If removing an insight would lose information not available elsewhere in the report, it's data, not an insight

### 5. Thresholds
- Every threshold: named constant + documented rationale + actual value shown in message
- Make thresholds configurable where user expectations vary (see [#105](https://github.com/dvhthomas/gh-velocity/issues/105))

## Related

- [Command Output Shape](command-output-shape.md) — four-layer output architecture (stats, detail, insights, provenance)
- [Complete JSON Output for Agents](complete-json-output-for-agents.md) — structured errors and warnings in JSON
- [Pipeline-per-Metric Architecture](../architecture-refactors/pipeline-per-metric-and-preflight-first-config.md) — directory-per-metric layout with render.go + templates
- [#82](https://github.com/dvhthomas/gh-velocity/issues/82) — insights for all report sections
- [#97](https://github.com/dvhthomas/gh-velocity/issues/97) — scope/`-R` conflict warning
- [#105](https://github.com/dvhthomas/gh-velocity/issues/105) — configurable defect rate threshold
