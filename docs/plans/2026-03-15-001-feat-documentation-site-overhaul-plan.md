---
title: "feat: Documentation site overhaul for clarity, consistency, and completeness"
type: feat
status: completed
date: 2026-03-15
origin: docs/brainstorms/2026-03-15-documentation-overhaul-brainstorm.md
---

# Documentation Site Overhaul

## Overview

Page-by-page rewrite of the gh-velocity Hugo documentation site. The current docs are comprehensive in coverage but too terse, inconsistent in key areas, and assume familiarity with the tool's mental model. This plan rewrites every existing page and fills content gaps (missing command references, undocumented features, unexplained terminology) to make the site genuinely helpful for engineers and technical PMs encountering gh-velocity for the first time.

## Problem Statement

1. **Too terse** — pages assume readers already understand concepts like matchers, lifecycle, scope, and strategies. No progressive disclosure for different audiences.
2. **Inconsistent** — the code defaults `cycle_time.strategy` to `issue` while guides say "start with pr." Config requirement is stated three different ways. Token tables differ between pages.
3. **Incomplete** — the entire commands reference section is empty. `report` (the most-referenced command) has no page. `status my-week`, `status reviews`, `risk bus-factor` are fully implemented but undocumented. The `field:` matcher type is used in examples but missing from the reference.
4. **Misleading onboarding** — getting-started leads with `-R cli/cli` (remote repo), implying the tool requires `-R`. Most users will run it in their own repo.

## Proposed Solution

Systematic, phased rewrite organized by dependency:

1. **Phase 1: Foundation** — Shelly Says CSS, getting-started section, landing page
2. **Phase 2: Reference** — command reference pages, config reference fixes, metric reference enrichment
3. **Phase 3: Guides** — rewrite guides with inline concept summaries and cross-links
4. **Phase 4: Concepts & polish** — concepts review, examples validation, troubleshooting, final consistency pass

Key design decisions (see brainstorm: `docs/brainstorms/2026-03-15-documentation-overhaul-brainstorm.md`):
- **Audience**: both engineers and PMs, using progressive disclosure
- **Getting started**: local repo first, `-R` as alternative
- **Cycle time**: explain all three strategies, help user choose (no default recommendation)
- **Concepts**: hybrid placement — inline summaries in guides, link to full concepts pages
- **Commands reference**: auto-generated via `cmd/gendocs` from Cobra command definitions (enrich Go source where needed)
- **Shelly Says**: `> TIP:` blockquotes styled with CSS to show Shelly SVG icon

## Technical Approach

### Content architecture

The site structure stays the same (getting-started → guides → concepts → reference → examples). Changes are to content, not navigation.

**Command reference pages (auto-generated):**

The `cmd/gendocs` tool already generates command reference pages from Cobra `Short`, `Long`, and `Example` fields into `site/content/reference/commands/`. The infrastructure works — the gap is that some Go command definitions need richer documentation. Rather than hand-writing markdown pages, we enrich the Go source and run `task build:site`.

**Commands needing enriched Go docs:**

| File | Current state | What to add |
|------|--------------|-------------|
| `cmd/configshow.go` | Minimal Long | Explain what "resolved" means (defaults applied, env vars merged) |
| `cmd/configvalidate.go` | Minimal Long | Explain what validation checks (schema, matcher syntax, project URL format) |
| `cmd/flow.go` (parent) | Short only | Add Long explaining the "flow" group: speed metrics for how fast work moves |
| `cmd/status.go` (parent) | Short only | Add Long explaining the "status" group: current state of work right now |
| `cmd/quality.go` (parent) | Short + minimal Long | Add brief guidance on when to use quality subcommands |
| `cmd/risk.go` (parent) | Short only | Add Long explaining the "risk" group: structural risks in the codebase |

**Commands already well-documented in Go (no changes needed):**

`report`, `cycletime`, `leadtime`, `throughput`, `velocity`, `wip`, `myweek`, `reviews`, `busfactor`, `preflight`, `discover`, `configcreate`, `qualityrelease`

**Existing pages to rewrite (21 content pages):**

All content pages listed in brainstorm Section D. The commands `_index.md` keeps its "auto-generated" notice since that's now accurate again.

**Terminology requiring inline definitions on first use:**

Matcher, scope, lifecycle, strategy, effort, linking, provenance (see brainstorm Section C). Each gets a 1-2 sentence inline definition with a `relref` link to the relevant concepts page.

### Institutional learnings to incorporate

These patterns from `docs/solutions/` must be reflected in the rewrite:

1. **Preflight-first onboarding** (`docs/solutions/evidence-driven-preflight-config.md`) — the getting-started configuration page must walk through the evidence block in generated configs, showing how to read match counts and identify weak matchers.

2. **Three-state metric output** (`docs/solutions/three-state-metric-status-pattern.md`) — "Interpreting Results" and "Agent Integration" must explain N/A (no start signal), in progress (started but not done), and duration (completed). JSON consumers need to know about `started_at` and `cycle_time_seconds` fields.

3. **Four-layer output shape** (`docs/solutions/architecture-patterns/command-output-shape.md`) — "Interpreting Results" should explain that commands produce stats, detail, insights, and provenance layers. "Agent Integration" should document the JSON structure per layer.

4. **Cycle time signal hierarchy** (`docs/solutions/cycle-time-signal-hierarchy.md`) — the cycle-time reference page needs a "How signals are resolved" section covering all 5 priority levels (label > project status > PR created > first assigned > first commit) plus backlog suppression.

5. **CLI hierarchy intent** (`docs/solutions/architecture-refactors/cobra-command-hierarchy-thematic-grouping.md`) — "How It Works" or the commands reference index should explain the organizing principle: flow = speed, status = now, quality = quality, risk = structural risk, config = setup.

6. **JSON error contract** (`docs/solutions/architecture-patterns/complete-json-output-for-agents.md`) — "Agent Integration" must document the stderr error envelope format and that warnings are embedded in JSON payload.

### Shelly Says implementation

Authors write `> TIP:` in markdown. CSS targets these blockquotes to prepend the Shelly SVG and style the callout distinctively.

**Implementation in** `site/layouts/_partials/docs/inject/head.html`:
- CSS selector targeting blockquotes whose first child paragraph starts with "TIP:"
- `::before` pseudo-element with Shelly SVG (similar to existing h1 turtle icon pattern)
- Distinct background color and border to differentiate from regular blockquotes

**Prototype this before writing content** to verify Hugo/hugo-book renders `> TIP:` in a CSS-targetable way.

### Implementation Phases

#### Phase 1: Foundation (getting-started + landing page + Shelly Says CSS)

**Why first:** Everything else links back to getting-started. The onboarding flow must be solid before guides can reference it.

**1a. Shelly Says CSS prototype**
- File: `site/layouts/_partials/docs/inject/head.html`
- Add CSS rules for `> TIP:` blockquote styling with Shelly SVG
- Test with a sample `> TIP:` block on an existing page
- Verify rendering in Hugo dev server (`task site`)

**1b. Quick Start rewrite** (`site/content/getting-started/quick-start.md`)
- Lead with local repo: `cd your-repo && gh velocity config preflight --write`
- First metric command: `gh velocity report --since 30d` (gracefully degrades, works without releases)
- Explain what the output means (brief, with link to Interpreting Results)
- Add `-R` as "You can also analyze repos you haven't cloned" secondary section
- Add Shelly Says callouts explaining "how did it know?" moments

**1c. How It Works rewrite** (`site/content/getting-started/how-it-works.md`)
- Keep the workflow tabs (solo/team/team-no-board) — they work well
- Add inline definitions for lifecycle, strategy, scope on first use
- Expand metric overview: each metric gets 2-3 sentences on what it measures and why
- Add CLI hierarchy explanation: flow = speed, status = now, quality = quality, risk = structural risk
- Link to each metric's reference page

**1d. Configuration rewrite** (`site/content/getting-started/configuration.md`)
- Restructure around decisions: scope → lifecycle → quality → velocity → cycle time
- Each section: what is it (1-2 sentences), example config, link to reference
- Walk through the preflight evidence block — show how to read match counts
- Canonical config requirement statement: "All metric commands require a `.gh-velocity.yml` file. Run `gh velocity config preflight --write` to generate one."
- Add `field:` matcher type to the matcher type list
- Consolidate token permissions to one canonical table (remove duplicate in ci-setup)

**1e. CI Setup review** (`site/content/getting-started/ci-setup.md`)
- Walk through simplest workflow first, then variations
- Link to canonical token table in configuration page (remove inline duplicate)
- Add guidance on which workflow to pick based on goals

**1f. Landing page** (`site/content/_index.md`)
- Rewrite "In a hurry?" snippet to use local repo flow
- First command: `gh velocity report --since 30d` (not `quality release -R cli/cli`)
- Keep it brief — 3 commands max

#### Phase 2: Reference (command docs enrichment + config fixes + metric enrichment)

**Why second:** Guides need to link to accurate reference pages. Build the reference layer before rewriting guides.

**2a. Enrich Go command definitions** (6 files needing work — see table above)
- Add/improve `Long` descriptions in `cmd/configshow.go`, `cmd/configvalidate.go`, `cmd/flow.go`, `cmd/status.go`, `cmd/quality.go`, `cmd/risk.go`
- Parent commands (`flow`, `status`, `quality`, `risk`) should explain the group's purpose and list what questions each subcommand answers
- Keep documentation in Go source as the single source of truth

**2b. Generate and verify command reference pages**
- Run `task build:site` to regenerate all command reference pages via `cmd/gendocs`
- Verify generated pages render correctly in Hugo dev server (`task site`)
- Update `site/content/reference/commands/_index.md` to add a brief intro explaining the command hierarchy before the auto-generated child list

**2c. Config reference fixes** (`site/content/reference/config.md`)
- Add `field:Name/Value` to matcher syntax table with examples
- Fix defaults documentation (verify against `internal/config/config.go`)
- Ensure every config key has at least one example
- Clarify that `cycle_time.strategy` default is `issue` without recommending one strategy over another

**2d. Metric reference enrichment** (`site/content/reference/metrics/*.md`)
- Each metric page: add "What this tells you" section (1-2 paragraphs for PMs)
- Cycle time: add signal hierarchy section (5 levels + backlog suppression)
- Velocity: clarify effort units (points vs items vs numeric)
- All: ensure examples use consistent config that matches current schema
- Cycle time: fix duplicate lifecycle block YAML example

**2e. API consumption review** (`site/content/reference/api-consumption.md`)
- Verify cost estimates are current
- Add entries for any commands missing from the table

#### Phase 3: Guides (rewrite with inline concepts and cross-links)

**Why third:** Now that reference pages exist, guides can link to them properly.

**3a. Interpreting Results** (`site/content/guides/interpreting-results.md`)
- Add "What do these numbers mean?" content for PMs
- Document three-state metric output (N/A, in progress, duration) with examples
- Add four-layer output shape explanation (stats, detail, insights, provenance)
- Show how output adapts to context (terminal → pretty, pipe → TSV, `--format json` → structured)
- Shelly Says callouts for common "why does it say N/A?" scenarios

**3b. Cycle Time Setup** (`site/content/guides/cycle-time-setup.md`)
- Replace "start with pr" recommendation with a decision tree:
  - Issues with labels → issue strategy
  - PRs as work units → PR strategy
  - GitHub Projects board → project-board strategy
- Inline summary of signal hierarchy with link to reference
- Shelly Says: "No labels yet? The PR strategy works without any setup."

**3c. Velocity Setup** (`site/content/guides/velocity-setup.md`)
- Clarify effort strategies: count (just count items), attribute (labels like `size:M`), numeric (project field)
- Explain iteration strategies: project-field, fixed, none
- Inline concept summary for effort and linking
- Link to velocity reference and config reference

**3d. Posting Reports** (`site/content/guides/posting-reports.md`)
- Clearly explain `--post` vs `--new-post`
- Explain GH_VELOCITY_POST_LIVE safety guard
- Show examples for each posting target (issue comment, discussion, release notes)

**3e. Agent Integration** (`site/content/guides/agent-integration.md`)
- Document four-layer JSON output shape with examples
- Document error envelope format on stderr
- Document three-state metric values in JSON
- Show how to parse `--format json` output programmatically

**3f. Recipes** (`site/content/guides/recipes.md`)
- Ensure each recipe is self-contained (brief context + command + explanation)
- Add recipe for `--scope` flag interaction with config scope (AND semantics)
- Add recipe for `status my-week` as 1:1 prep

**3g. Troubleshooting** (`site/content/guides/troubleshooting.md`)
- Add `--debug` usage guide: what it shows (search queries, API calls, strategy resolution, timing)
- Add section on zero-match matchers and how to diagnose with preflight evidence
- Add section on `preflight --write` overwrite behavior

#### Phase 4: Concepts, examples, and consistency pass

**4a. Concepts review** (`site/content/concepts/*.md`)
- GitHub Capabilities: review for clarity
- Statistics: light touch (reportedly strong)
- Linking Strategies: add clear "when to use each" guidance per strategy
- Labels vs Board: ensure `field:` matcher is properly documented with examples

**4b. Examples validation** (`site/content/examples/_index.md`)
- Validate every YAML example against current config schema (`internal/config/config.go`)
- Fix any examples using outdated keys or patterns
- Add `description` frontmatter to improve `children` shortcode output if examples are split

**4c. Final consistency pass**
- Search for all instances of "start with pr" or similar cycle time recommendations → fix
- Search for all config requirement statements → align to canonical phrasing
- Verify all `relref` links resolve (Hugo build will catch broken ones)
- Verify token tables are consolidated (one location, others link)
- Check that every term from the terminology list (Section C) has an inline definition on first use in each section

## Acceptance Criteria

### Functional Requirements

- [x] Every implemented command has an auto-generated reference page in `reference/commands/` (via enriched Go docs + `task build:site`)
- [x] Quick-start flow starts with local repo, uses `report --since 30d` as first command
- [x] `field:Name/Value` matcher type documented in config reference and configuration page
- [x] Cycle time docs explain all three strategies without recommending one default
- [x] Config requirement stated consistently across all pages: "All metric commands require a `.gh-velocity.yml`"
- [x] Token permissions table exists in one canonical location; other pages link to it
- [x] Three-state metric output (N/A, in progress, duration) documented in Interpreting Results
- [x] Four-layer output shape (stats, detail, insights, provenance) documented in Interpreting Results and Agent Integration
- [x] Cycle time signal hierarchy (5 levels + backlog suppression) documented in cycle-time reference
- [x] Shelly Says (`> TIP:`) callouts render with Shelly SVG styling
- [x] All terminology from brainstorm Section C has inline definitions on first use per section
- [x] Commands reference `_index.md` has a brief intro explaining the command hierarchy
- [x] Landing page "In a hurry?" uses local repo flow

### Non-Functional Requirements

- [x] Hugo site builds without warnings (`task build:site`)
- [x] No broken `relref` links (Hugo build catches these)
- [x] Writing style is direct, friendly, and uses progressive disclosure
- [x] Pages are as long as they need to be and no longer (not verbose for verbosity's sake)

## Dependencies & Prerequisites

- Hugo dev server working (`task site`) for live preview during editing
- Access to `cmd/*.go` files to source command descriptions, flags, and examples
- Access to `internal/config/config.go` to verify schema and defaults
- The Shelly SVG asset at `site/assets/favicon.svg` (already exists)

## Risk Analysis & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Shelly Says CSS doesn't work with Hugo's blockquote rendering | Medium | Low | Prototype in Phase 1a before writing content. Fallback: use a Hugo shortcode. |
| Page rewrites introduce new inconsistencies | Medium | Medium | Phase 4c is a dedicated consistency pass. Hugo build catches broken links. |
| Command reference pages drift from code | Low | Low | Auto-generated from Go source via `cmd/gendocs`. Running `task build:site` always produces current docs. |
| Scope creep into code changes | Low | High | Brainstorm explicitly says code changes are out of scope. Note issues separately. |

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-15-documentation-overhaul-brainstorm.md](docs/brainstorms/2026-03-15-documentation-overhaul-brainstorm.md) — Key decisions: local-repo-first onboarding, dual audience with progressive disclosure, explain-don't-recommend for cycle time, hand-written command reference, Shelly Says callout pattern.

### Internal References

- Evidence-driven preflight config: `docs/solutions/evidence-driven-preflight-config.md`
- Three-state metric pattern: `docs/solutions/three-state-metric-status-pattern.md`
- Command output shape: `docs/solutions/architecture-patterns/command-output-shape.md`
- Cycle time signal hierarchy: `docs/solutions/cycle-time-signal-hierarchy.md`
- CLI hierarchy design: `docs/solutions/architecture-refactors/cobra-command-hierarchy-thematic-grouping.md`
- JSON error contract: `docs/solutions/architecture-patterns/complete-json-output-for-agents.md`
- Config schema: `internal/config/config.go`
- Hugo site config: `site/hugo.toml`
- Custom CSS: `site/layouts/_partials/docs/inject/head.html`

### Hugo-book Theme Resources

- Available shortcodes (unused): `tabs`, `steps`, `details`, `columns`, `button`, `badge`, `card`, `mermaid`
- Markdown alerts (already used): `> [!NOTE]`, `> [!WARNING]`
- Custom shortcodes (in use): `asset-img`, `children`
