# Documentation Overhaul Brainstorm

**Date:** 2026-03-15
**Status:** Completed

## What We're Building

A thorough, page-by-page rewrite of the gh-velocity documentation site to make it genuinely helpful for engineers and technical PMs who are new to the tool. The current docs are comprehensive in coverage but too terse, inconsistent in places, and assume too much familiarity with the tool's mental model.

### Goals

1. **Helpful for newcomers** — an engineer or technical PM should be able to go from "what is this?" to "running in CI" without confusion
2. **Internally consistent** — terminology, recommendations, and examples should not contradict each other across pages
3. **Clear and direct** — friendly tone, progressive disclosure, no unexplained jargon

### Non-goals

- Volume of documentation is not a goal. Pages should be as long as they need to be and no longer.
- Redesigning the site navigation or theme.
- Changing the tool's actual behavior (code changes are out of scope, though we may note bugs/inconsistencies to fix separately).

## Why This Approach

**Page-by-page rewrite** was chosen over reader-journey or fix-list approaches because:

- The "too terse" and "assumes familiarity" problems are pervasive, not isolated to specific pages
- Consistency requires touching every page anyway to align terminology and cross-references
- A systematic pass ensures nothing falls through the cracks

## Key Decisions

### 1. Audience: both engineers and technical PMs

Use progressive disclosure. Quick practical steps up front for engineers who want to get running. Deeper "what does this number mean?" explanations for PMs who need to interpret results and communicate them to stakeholders. Flag audience-specific sections where needed.

### 2. Getting started leads with local repo

The current getting-started flow uses `-R cli/cli` (a remote public repo), which implies the tool is repo-centric and requires `-R`. The rewrite starts from "cd into your repo, run preflight, get your config, run a command." Remote repos introduced as an alternative later. This matches how most people will actually use the tool.

### 3. Cycle time strategy: explain both, don't pick a default recommendation

The code defaults to `issue` strategy, but guides currently say "start with pr." Neither is universally better. The docs should help users choose based on their workflow:
- **PR strategy**: works out of the box, measures PR open-to-merge time, good if PRs are your unit of work
- **Issue strategy**: richer signal (uses labels for lifecycle stages), good if issues are your unit of work, requires label discipline
- **Project board strategy**: uses board column transitions, good for teams already using GitHub Projects

The "How It Works" tabs already partially do this — extend that pattern.

### 4. Hybrid concept placement

Guides include brief inline explanations (1-2 sentences) of key concepts with "learn more" links to the full concepts pages. The standalone Concepts section stays as the canonical reference for "why" and "how things work." This avoids duplication while giving readers context right when they need it.

### 5. Auto-generated command reference pages (revised)

~~Originally planned as hand-written, revised to use auto-generation.~~ The `cmd/gendocs` tool already generates command reference pages from Cobra `Short`, `Long`, and `Example` fields. The infrastructure works — the gap is that some Go command definitions need richer documentation. Enrich the Go source (single source of truth) and run `task build:site` to generate pages. Simpler, no drift.

## Detailed Findings and Fixes

### A. Structural Issues

| Issue | Location | Fix |
|-------|----------|-----|
| Empty commands reference | `reference/commands/` | Write pages for: `report`, `quality release`, `flow lead-time`, `flow cycle-time`, `flow throughput`, `flow velocity`, `status wip`, `config preflight`, `config discover`, `config validate`, `config show`, `version` |
| `report` command referenced everywhere but never explained | Landing page, CI setup, interpreting results, recipes, agent integration | Write a dedicated command reference page; update getting-started to introduce it properly |
| `field:` matcher type missing from config reference | `reference/config.md` matcher syntax section | Add `field:Name/Value` to the matcher syntax documentation |
| Bus factor and reviews mentioned in token tables but undocumented | `getting-started/configuration.md`, `getting-started/ci-setup.md` | Write metric reference pages (even if brief) or remove references if these features aren't ready |
| WIP command referenced but undocumented | `api-consumption.md`, `configuration.md`, `labels-vs-board.md`, `how-it-works.md` | Write a command reference page or clearly mark as upcoming |

### B. Consistency Fixes

| Issue | Pages affected | Fix |
|-------|---------------|-----|
| Config requirement contradictions | quick-start (requires config), configuration.md ("only if you want to change something"), config.md (required) | Align on one clear statement: config is required for all metric commands, preflight generates a starter config |
| Cycle time default mismatch | `reference/config.md` (default: issue), `how-it-works.md` and `cycle-time-setup.md` ("start with pr") | Remove default recommendation; explain both strategies and help user choose (per decision #3) |
| Token permission tables differ | `configuration.md` vs `ci-setup.md` | Consolidate to one canonical table in one location, link from the other |
| Quick-start runs commands without `--config` | `getting-started/quick-start.md` | Fix examples to either include config or explain why it's not needed for that specific case |
| Duplicate lifecycle blocks in cycle-time reference | `reference/metrics/cycle-time.md` | Restructure the YAML examples to be clear about which parts are alternatives vs. complementary |

### C. Terminology and Clarity

Terms that need inline definitions or a glossary entry on first use:

- **Matcher** — a pattern like `label:bug` or `field:Status/Done` that classifies issues/PRs. Currently explained only deep in the config reference.
- **Scope** — the filter that determines which issues/PRs are included. Needs to clearly distinguish config `scope.query` from the `--scope` CLI flag and explain they're AND'd together.
- **Lifecycle** — the label-based or board-based stages an issue moves through (e.g., triage → in-progress → done). Core concept, often assumed.
- **Strategy** — how the tool decides which data source to use for a metric (e.g., pr-link, commit-ref, changelog for linking; issue, pr, project-board for cycle time).
- **Effort** — the sizing/point value assigned to issues for velocity calculation. Can come from labels or project board fields.
- **Linking** — how the tool connects issues to code changes (PRs, commits, releases). Three strategies exist.
- **Provenance** — metadata showing where a metric's data came from (which API calls, which config settings).

### D. Page-by-Page Rewrite Plan

#### Getting Started Section

1. **Quick Start** — Rewrite to start with local repo. Steps: install → cd into repo → `gh velocity config preflight --write` → explain what the generated config contains → run first metric command → see results. Introduce `-R` as "you can also analyze remote repos."

2. **How It Works** — Keep the workflow tabs (solo/team/team-no-board). Add inline definitions for lifecycle, strategy, scope. Expand the metric overview so each metric gets 2-3 sentences explaining what it measures and why you'd care, not just a name.

3. **Configuration** — Restructure around "what decisions do I need to make?" rather than "here's every config key." Walk through: scope (what repos/issues), lifecycle (how do you track progress), quality (what categories matter), velocity (how do you size work), cycle time (which strategy fits). Each section: what it is (1-2 sentences), example config, link to full reference.

4. **CI Setup** — Consolidate token table to one place. Walk through the simplest CI setup first, then variations. Currently shows four workflows without clear guidance on which to pick.

#### Guides Section

5. **Interpreting Results** — Add more "what does this number mean in practice?" content. Currently shows output format but doesn't help a PM interpret whether their numbers are good/bad/normal.

6. **Velocity Setup** — Clarify the relationship between effort config, categories, and the velocity metric. Add inline concept summaries.

7. **Cycle Time Setup** — Remove "start with pr" recommendation. Replace with a decision tree: "If your team tracks work via issues with labels → issue strategy. If PRs are your unit of work → PR strategy. If you use GitHub Projects → project-board strategy."

8. **Posting Reports** — Review for clarity. Explain `--post` vs `--new-post` clearly.

9. **Agent Integration** — Review for clarity.

10. **Recipes** — Review for clarity. Ensure each recipe is self-contained with enough context.

11. **Troubleshooting** — Review for completeness.

#### Concepts Section

12. **GitHub Capabilities** — Review for clarity.

13. **Statistics** — This is reportedly strong. Light touch review.

14. **Linking Strategies** — Ensure the three strategies are clearly distinguished with "when to use each" guidance.

15. **Labels vs Board** — Document the `field:` matcher type properly here and in config reference.

#### Reference Section

16. **Metric reference pages** (lead-time, cycle-time, velocity, throughput, quality) — Review each for accuracy and add practical interpretation guidance.

17. **Config reference** — Add `field:` matcher syntax. Fix defaults documentation. Ensure every config key has an example.

18. **API consumption** — Review for accuracy.

19. **Command reference pages** — Write new pages for all commands (see section A above).

#### Other

20. **Landing page** — Rewrite the quick-start snippet to use local repo flow.

21. **Examples page** — Review example configs for accuracy against current config schema.

### 6. "Shelly Says" callout pattern

Shelly (the mascot) can provide friendly asides throughout the docs — explanations of "how did it know that?" or tips that add personality. These use blockquote syntax with a "Shelly Says" prefix that can be detected and styled with a Shelly SVG icon. Example use cases:
- Quick-start: "How did Shelly know which labels to look for? That's what `preflight` does — it reads your repo and suggests config based on what it finds."
- Configuration: "Shelly says: the `-R` flag tells me which repo to look at. If you're already in the repo directory, I'll figure it out!"
- Cycle time: "Shelly says: no labels yet? The PR strategy works without any setup — it uses your merge times."

Implementation: Authors write `> TIP: ...` in markdown. CSS content replacement transforms the `TIP:` prefix into `<svg> Shelly Says:` with the Shelly SVG icon. Simple, no shortcodes needed — just a CSS rule targeting blockquotes that start with "TIP:".

## Open Questions

*None — all questions resolved during brainstorming.*

## Resolved Questions

1. **Audience?** Both engineers and technical PMs equally, using progressive disclosure.
2. **Getting started flow?** Local repo first, remote repos as alternative.
3. **Scope?** Full pass including new pages for undocumented features.
4. **Cycle time recommendation?** Explain both strategies, help user choose based on workflow.
5. **Concept placement?** Hybrid — inline summaries in guides, link to full concepts pages.
6. **Commands reference?** Hand-written pages, not auto-generated.
