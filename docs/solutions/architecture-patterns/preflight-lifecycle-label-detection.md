---
title: "Preflight lifecycle label detection gaps"
category: "architecture-patterns"
date: "2026-03-17"
tags:
  - preflight
  - cycle-time
  - labels
  - lifecycle
  - config-generation
module: "cmd/preflight.go"
severity: "moderate"
---

# Preflight Lifecycle Label Detection Gaps

## Problem

Three of 9 showcase repos had no cycle time data despite having usable lifecycle labels or better strategy options. The preflight command's label detection is too strict — it misses common label variants.

## Root Cause

1. **Exact string matching**: Preflight searches for `in-progress` (hyphenated) but hashicorp/terraform uses `in progress` (space). Punctuation variants like `review me!` are also missed.
2. **No pipeline detection**: github/roadmap uses a label progression (`exploring → in design → preview → ga → shipped`) that could be auto-detected as lifecycle stages, but preflight only looks for individual known labels.
3. **Wrong strategy fallback**: grafana/k6 has no lifecycle labels but uses PRs to close issues. Preflight chose `strategy: issue` instead of `strategy: pr`.

## Research Findings

| Repo | Useful Labels Found | Preflight Detected |
|------|--------------------|--------------------|
| hashicorp/terraform | `in progress`, `confirmed`, `review me!`, `new` | Only `backlog` (from `priority/backlog`) |
| github/roadmap | `in design`, `preview`, `Public Preview`, `ga`, `shipped` | Only `backlog` (from `Resolution: Backlog`) |
| grafana/k6 | None — workflow uses assignees and linked PRs | `wip` (stale-bot label, not lifecycle) |

## Solution

Filed as [#109](https://github.com/dvhthomas/gh-velocity/issues/109) with proposed improvements:

1. **Fuzzy label matching**: normalize case, separators (`-`, ` `, `_`), strip trailing punctuation (`!`, `?`)
2. **Keyword-based stage mapping**: match `progress`, `review`, `confirmed`, `design`, `blocked`, `shipped` etc. to lifecycle stages
3. **Pipeline detection**: when multiple lifecycle labels exist, suggest them as a progression
4. **Strategy fallback**: when no lifecycle labels found but PRs reference issues, suggest `strategy: pr`

## Prevention

- When adding label detection patterns, test against real repos with varied naming conventions
- Maintain a keyword → lifecycle-stage mapping table as a reference for what to detect
- The showcase serves as an integration test for preflight — review its output for missing data

## Related

- [#109](https://github.com/dvhthomas/gh-velocity/issues/109) — preflight fuzzy label matching issue
- [Render-Layer Linking](render-layer-linking-and-insight-quality.md) — related quality drive documentation
- [Discussion #107](https://github.com/dvhthomas/gh-velocity/discussions/107) — showcase run that exposed the gaps
