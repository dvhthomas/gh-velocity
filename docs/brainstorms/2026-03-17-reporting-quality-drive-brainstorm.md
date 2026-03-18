# Brainstorm: Reporting Quality Drive

**Date:** 2026-03-17
**Status:** brainstorm
**Triggered by:** Showcase run ([Discussion #96](https://github.com/dvhthomas/gh-velocity/discussions/96)) revealed 14 UX/quality issues across all output formats.

## What We're Building

A single quality sweep across reporting output to fix three categories of problems visible in real showcase data:

1. **Noise detection & data quality** — spam/invalid issues distort stats (cli/cli median lead time is 1 minute because ~50 spam issues close in seconds, producing absurd "76798x longer than the median" messages).
2. **Links & discoverability** — stat names (CV, P90), issue references (#12730), and metric names (Lead Time) in output should link to docs and GitHub issues respectively, so readers can click to learn more.
3. **Missing output sections** — Quality has no detail section in reports; Throughput detail is bare; cycle time detail tables show N/A rows for items without data.

## Why This Approach

The showcase is the tool's public demo. Every UX issue in the showcase output is a credibility problem — "76798x longer" makes the tool look broken even though the math is correct. These fixes are all rendering/insight-layer changes: no API changes, no new data fetching, no model changes beyond adding URL to ItemRef. They can ship as one coherent PR or a small series.

## Key Decisions

### 1. Noise detection: insight only, no auto-exclusion

When many issues close in under 60 seconds, surface an insight like:
> 54 issues closed in under 60 seconds (likely spam or automation). Consider narrowing scope: `scope: '-label:invalid -label:suspected-spam'`

No silent filtering. The user decides what to exclude via config. This respects the principle that the tool shows what it sees and suggests what to do.

### 2. Docs base URL: hardcoded in one constant

A single `DocSiteURL` constant (e.g., in `internal/format/links.go`). All doc links reference it. No config key. YAGNI — there's one canonical docs site.

### 3. Issue links in insights: add URL to ItemRef

`ItemRef` gets a `URL` field. The insight generator embeds `[#N](url)` links directly in markdown messages. The data is already available at the call site — just needs propagation.

### 6. Links are format-aware

Doc links and issue links only render as markdown `[text](url)` in markdown format. In pretty-text output, they render as plain text (no URLs — terminal noise). In JSON output, insight messages contain the markdown links (JSON consumers can parse or strip them), and doc/issue URLs appear as separate fields where useful.

### 4. Insight deduplication: keep status quo

Key Findings and detail sections intentionally duplicate insights. Each section should stand alone when viewed in isolation (e.g., as a separate artifact file or when expanded individually in a discussion post).

### 5. Outlier multiplier: cap at 100x

When the outlier cutoff / median ratio exceeds 100, display "100x+" instead of the exact number. This prevents absurd messages like "76798x" while still communicating "way more than normal."

Additionally, when median < 1 hour AND count > 10, add a diagnostic insight: "Median lead time is N minutes — likely distorted by noise issues" to flag that the median itself is unreliable.

## Scope: All Changes

### Theme 1: Noise Detection & Data Quality

| ID | Issue | Change |
|----|-------|--------|
| a | Sub-60s issues distort stats | New `noise_detection` insight in `GenerateStatsInsights`: count items < 60s, suggest scope exclusion if >= 3 |
| g | Absurd outlier multiplier (76798x) | Cap multiplier at 100x ("100x+"). Add low-median diagnostic when median < 1h and n > 10 |
| h | Cycle time detail shows N/A rows | Filter detail table to items with cycle time data. Update `<summary>` count to reflect filtered count |
| i | Fastest/slowest picks spam items | Skip items with duration < 1 minute in `findExtremes` |
| j | 78% defect rate from template titles | New `defect_rate_review` insight when defect rate > 60%: "78% defect rate may reflect issue template naming rather than actual bugs — review category matchers" |
| k | Extreme absolute values unremarked | New insight when median lead time > 365 days: "Median lead time exceeds 1 year — likely includes backlog cleanup" |

### Theme 2: Links & Discoverability

| ID | Issue | Change |
|----|-------|--------|
| b | Stat names (CV, P90) unlinked | Wrap stat names in insight messages with doc links: `[CV](docs/concepts/statistics/#...)`, `[P90](docs/concepts/statistics/#...)` |
| c | Issue numbers in insights unlinked | Add `URL` field to `ItemRef`. Update `fastest_slowest` and `outlier_detection` insights to use `[#N](url)` |
| f | Metric names in summary table unlinked | In report.md.tmpl, change `Lead Time` to `[Lead Time](docs/reference/metrics/lead-time/)` etc. |
| e | Search URLs in summary table | Skip — client-side filtering makes these approximate at best. Not worth the complexity. |

### Theme 3: Missing Output Sections

| ID | Issue | Change |
|----|-------|--------|
| d | No Quality detail section in report | Add `<details>` block for Quality: per-item table (issue, category, lead time), defect rate breakdown by category, hotfix list |
| m | Throughput detail is bare | Add category breakdown table to throughput detail section (already computed as an insight — move to a table) |
| n | No WIP row when unconfigured | Show `WIP | not configured` row with link to setup docs when WIP data is nil but other sections have data |

### Not doing

| ID | Issue | Reason |
|----|-------|--------|
| e | Search URLs in summary table | Client-side filtering means URLs wouldn't match. Low value. |
| l | Deduplicate Key Findings vs detail | Decided to keep duplication — sections should be self-contained. |

## Implementation Order

**Phase 1: Data quality (P1 issues, highest impact)**
- (a) noise detection insight
- (g) cap outlier multiplier + low-median diagnostic
- (h) filter N/A rows from cycle time detail
- (i) min-duration threshold for fastest/slowest

**Phase 2: Links (P2, high polish)**
- Add `DocSiteURL` constant
- (b) stat name links in insights
- (c) URL on ItemRef + issue links in insights
- (f) metric name links in report table

**Phase 3: Missing sections (P2-P3, completeness)**
- (d) quality detail section
- (j) defect rate sanity check insight
- (k) extreme absolute value insight
- (m) richer throughput detail
- (n) explicit "not configured" for WIP

## Files to Change (Preliminary)

| File | Phases | What |
|------|--------|------|
| `internal/metrics/report_insights.go` | 1, 3 | Noise insight, multiplier cap, fastest/slowest filter, defect sanity, extreme median |
| `internal/metrics/report_insights_test.go` | 1, 3 | Tests for all new insight rules |
| `internal/format/links.go` | 2 | `DocSiteURL` constant, `DocLink(anchor)` helper |
| `internal/format/report.go` | 2, 3 | Stat name links, WIP "not configured" row |
| `internal/format/templates/report.md.tmpl` | 2, 3 | Metric name links, Quality detail, WIP row |
| `internal/format/templates.go` | 2 | Add `docLink` to funcMap |
| `internal/pipeline/cycletime/render.go` | 1 | Filter N/A rows from detail table |
| `internal/pipeline/cycletime/templates/cycletime-bulk.md.tmpl` | 1 | Filtered count in summary |
| `cmd/report.go` | 3 | Wire quality detail section, WIP "not configured" |
| `internal/metrics/report_insights.go` | 2 | ItemRef.URL, linked issue references |

## Open Questions

None — all key decisions resolved during brainstorm.

## Verification

After each phase:
1. `go test ./... -count=1` — all pass
2. Build binary, run against cli/cli (the worst-case repo for noise)
3. Run showcase workflow, review discussion output
4. Verify links render correctly in GitHub markdown
