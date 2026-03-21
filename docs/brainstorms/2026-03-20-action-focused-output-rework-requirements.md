---
date: 2026-03-20
topic: action-focused-output-rework
---

# Action-Focused Output Rework

## Problem Frame

Report output across gh-velocity commands is inconsistent and data-heavy rather than action-driving. WIP was recently reworked (lipgloss tables, signal-first sorting, 25-item cap) and is the gold standard. Every other command still uses the old tableprinter, dumps up to 1000 unsorted items, and lacks a unified signal system. Doc links in markdown output point to pages that may not exist, making "learn more" links frustrating dead ends.

The result: readers scan past walls of data instead of acting on what matters.

## Requirements

### Uniform detail tables

- R1. All pretty-format tables use lipgloss/table (rounded borders, bold headers, right-aligned numbers). Remove go-gh tableprinter dependency from all output rendering.
- R2. Detail tables in pretty and markdown are capped at 25 items. JSON output is always uncapped. When capped, show a note: "Showing 25 of N items (sorted by signal). Use `--format json` for full data."
- R3. Detail tables sort signal-first: items needing attention (outliers, stale, noise) sort to the top, then by duration descending within each signal tier. Same mental model as WIP's needs-attention sort.
- R4. Signal column is always the first column in detail tables (matching WIP pattern).

### Unified signal system

- R5. Standardize emoji signals across all commands: 🚩 outlier, 🤖 noise, ⚡ hotfix, 🐛 bug, ⏳ stale, 🟡 aging. Replace text labels ("STALE", "OUTLIER") with the unified set.
- R6. Signal definitions are shared constants (not per-command). Each command uses the subset that applies to its domain.

### Doc link validation

- R7. Validate all `DocLink()` anchor targets against actual doc site headings. Fix or remove broken anchors. Treat doc heading anchors as a public API — changing them requires updating all references.
- R8. Add a CI check or build-time test that verifies doc link anchors resolve to real headings in the docs site content.

### Provenance everywhere

- R9. All commands include provenance (command invoked + key config values) in all formats. In pretty: collapsed or footer. In markdown: `<details>` block. In JSON: `"provenance"` field.

### Format completeness

- R10. Quality pipeline adds JSON output (currently only pretty and markdown).
- R11. Every command supports all three core formats: json, pretty, markdown. HTML remains report-only.

### Content quality

- R12. Every insight must contain a judgment or comparison — not restate data visible in tables (per existing insight-vs-data pattern).
- R13. Commands currently missing insights (WIP, reviews, release, issue detail, pr detail) get at least one actionable insight when data supports it.

## Success Criteria

- Running any command with `--results pretty` produces a lipgloss table, sorted signal-first, capped at 25, with a unified emoji signal column.
- Running any command with `--results md` produces markdown with doc links that all resolve to real pages.
- Running any command with `--results json` produces uncapped data with provenance.
- A reader can look at the first 5 rows of any detail table and see the items most worth investigating.
- No command produces more than a screenful of detail in pretty without the user opting in.

## Scope Boundaries

- **Not doing:** New commands, new metrics, new data fetching, new config keys.
- **Not doing:** `--limit` flag for user-configurable caps (may come later, but 25 is the default for now).
- **Not doing:** `--sort` flag for user-configurable sort order.
- **Not doing:** Trend arrows or period-over-period comparison (separate feature).
- **Not doing:** Changing JSON schema structure beyond adding provenance and quality JSON.

## Key Decisions

- **25-item cap:** Tight focus. Forces sorting to matter. Very scannable. JSON remains uncapped for automation.
- **Signal-first sort:** Matches WIP pattern. Outliers and stale items bubble to top. Users see what needs attention without scrolling.
- **Lipgloss everywhere:** Uniform visual language. WIP already proved it works well.
- **Provenance on all commands:** Self-documenting, reproducible output. Low cost since the pattern exists in velocity.
- **Unified emoji signals:** Visual consistency. Same emoji means the same thing regardless of command.
- **Doc links: keep but validate:** Links add real value when they work. Fix broken ones, add CI guard.

## Dependencies / Assumptions

- Doc site content exists at `dvhthomas.github.io/gh-velocity` with predictable heading anchors.
- lipgloss/table dependency already in go.mod (used by WIP).
- Provenance pattern already implemented in velocity pipeline — can be extracted and reused.

## Outstanding Questions

### Deferred to Planning

- [Affects R3][Technical] What signal tiers apply to each command? Lead-time has outlier/noise/hotfix; quality has bug; release has outlier. Need to map the full signal-per-command matrix.
- [Affects R8][Needs research] What's the simplest way to validate doc link anchors in CI? Options: scrape the built site, parse markdown headings directly, or use a link checker tool.
- [Affects R9][Technical] How should provenance render in pretty format? Options: footer line, collapsed section, or always-visible block. Velocity uses a full section — may be too verbose for simple commands.
- [Affects R13][Needs research] What actionable insights make sense for commands currently lacking them (reviews, release, issue detail, pr detail)?

## Next Steps

→ `/ce:plan` for structured implementation planning
