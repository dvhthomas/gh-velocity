---
title: "Hugo Documentation Site for gh-velocity"
date: 2026-03-14
status: completed
type: brainstorm
---

# Brainstorm: Hugo Documentation Site

## What We're Building

A Hugo-powered documentation site in `site/` that becomes the definitive reference for gh-velocity. Three content pillars:

1. **Getting Started** — Install, first run, interpret your first report. Serves both individual devs and team leads.
2. **Task-based guides** — "Interpreting cycle time results", "Setting up CI", "Configuring velocity for your team", "Choosing an effort strategy". Answer "how do I...?" questions.
3. **Reference docs** — Complete config schema, CLI command reference, metric definitions (our specific definitions of cycle time, lead time, etc.), API consumption details. Auto-generated where possible.

The site replaces `docs/guide.md` by decomposing it into individual pages. `guide.md` stays in the repo as a deprecated pointer to the site. README shrinks over time but keeps install + quick example.

## Why This Approach

- **Hugo Book theme** — sidebar nav, search, clean layout. Professional immediately with minimal config. Matches the pattern from bitsby.me/til/2021-02-28/how-to-make-a-hugo-blog-from-scratch/ but with a docs-oriented theme.
- **Auto-generated CLI + config reference** — Cobra's `cobra/doc` generates markdown command pages. Config schema generated from Go struct tags. Keeps docs in sync with code without manual maintenance.
- **GitHub Pages** — zero infrastructure. Default URL (`dvhthomas.github.io/gh-velocity`) initially, custom domain later if desired.
- **`task docs:build` + GitHub Action** — builds on push to main, deploys to Pages. Matches existing Taskfile patterns.

## Key Decisions

1. **Location**: `site/` folder in the repo root
2. **Theme**: Hugo Book (github.com/alex-shpak/hugo-book)
3. **Audience**: Both individual devs and team leads/eng managers
4. **Migration**: Decompose `docs/guide.md` into individual site pages. guide.md becomes deprecated.
5. **Auto-generation**: CLI reference via `cobra/doc`, config schema from Go struct tags/comments
6. **Hosting**: GitHub Pages, default URL initially, custom domain deferred
7. **Build**: `task docs:build` locally, GitHub Action on push to main
8. **Content from existing material**: guide.md (56KB), README.md, solutions docs, example configs

## Site Structure

```
site/
├── hugo.toml
├── content/
│   ├── _index.md                    # Landing page
│   ├── getting-started/
│   │   ├── _index.md                # Install + first run
│   │   ├── quick-start.md           # 5-minute guide
│   │   ├── configuration.md         # First config via preflight
│   │   └── ci-setup.md              # GitHub Actions integration
│   ├── guides/
│   │   ├── _index.md
│   │   ├── interpreting-results.md  # How to read output
│   │   ├── velocity-setup.md        # Effort strategies, iterations
│   │   ├── cycle-time-setup.md      # Strategies, lifecycle config
│   │   ├── posting-reports.md       # --post, discussions, comments
│   │   ├── multi-repo.md            # Running across repos
│   │   └── troubleshooting.md       # Common issues, N/A results
│   ├── reference/
│   │   ├── _index.md
│   │   ├── metrics/
│   │   │   ├── _index.md            # Overview of all metrics
│   │   │   ├── lead-time.md         # Our definition + formula
│   │   │   ├── cycle-time.md        # Our definition + strategies
│   │   │   ├── velocity.md          # Effort, iterations, completion rate
│   │   │   ├── throughput.md        # Count-based flow metric
│   │   │   └── quality.md           # Defect rate, hotfix window
│   │   ├── commands/                # Auto-generated from Cobra
│   │   │   ├── _index.md
│   │   │   ├── gh-velocity-flow-velocity.md
│   │   │   ├── gh-velocity-flow-lead-time.md
│   │   │   └── ...
│   │   ├── config.md                # Auto-generated from Go structs
│   │   └── api-consumption.md       # Rate limits, budget, caching
│   └── examples/
│       ├── _index.md
│       └── configs.md               # Example configs with explanations
├── static/
│   └── images/                      # Screenshots, diagrams
├── layouts/                         # Overrides if needed
└── go.mod                           # Hugo module for theme
```

## Content Migration Map

| Source | Destination | Notes |
|--------|------------|-------|
| docs/guide.md § "Metric Definitions" | reference/metrics/*.md | Split into one page per metric |
| docs/guide.md § "Configuration Reference" | reference/config.md | Replace with auto-generated |
| docs/guide.md § "Getting Started" | getting-started/*.md | Expand into multi-page flow |
| docs/guide.md § "CI / GitHub Actions" | getting-started/ci-setup.md | |
| docs/guide.md § "Troubleshooting" | guides/troubleshooting.md | |
| docs/guide.md § "Token Permissions" | getting-started/configuration.md | Fold into config guide |
| docs/solutions/*.md | Internal only (not migrated) | Architecture decisions stay in docs/ |
| docs/examples/*.yml | examples/configs.md | Annotated examples |
| README.md § "What gets measured" | reference/metrics/_index.md | |
| README.md § "Config reference" | reference/config.md | Replace with auto-generated |

## Build Pipeline

1. **`task docs:generate`** — runs Go code to emit CLI reference (cobra/doc) and config schema markdown into `site/content/reference/commands/` and `site/content/reference/config.md`
2. **`task docs:build`** — runs `hugo --source site` to build the static site
3. **`task docs:serve`** — runs `hugo server --source site` for local preview
4. **GitHub Action** — on push to main: generate → build → deploy to Pages

## Auto-Generation Strategy: Go Docs as Source of Truth

Rather than a heavyweight code-generation pipeline, the goal is **well-written Go doc comments that get extracted with a simple preprocessor**.

### CLI Reference
Cobra's `cobra/doc` package generates markdown from command definitions. The Go code already has `Short`, `Long`, `Example` fields — make those excellent and the reference pages write themselves.

### Config Schema
Go struct tags + doc comments on config types are the source of truth. A simple Go program walks the config structs, reads field names/types/tags/comments, and emits a markdown reference page. No external schema language needed.

### Metric Definitions
Doc comments on metric computation functions (e.g., `computeLeadTime`, `computeCycleTime`) can include `// Definition:` blocks that a preprocessor extracts into reference pages. This keeps definitions next to the code that implements them.

### Preprocessor Approach
A single `cmd/gendocs/main.go` tool that:
1. Uses `cobra/doc` for CLI reference
2. Walks config struct fields via reflection + `go/ast` for doc comments
3. Optionally extracts tagged doc blocks from metric code
4. Writes markdown into `site/content/reference/`

Run via `task docs:generate` before `task docs:build`.

## Resolved Questions

1. **Audience**: Both devs and team leads — tone should be practical and direct, not academic.
2. **Theme**: Hugo Book — sidebar nav, search, professional out of the box.
3. **Migration strategy**: Decompose guide.md into pages. guide.md stays as deprecated pointer.
4. **Auto-generation**: CLI via cobra/doc, config schema from Go struct tags + doc comments, metric definitions from tagged doc blocks. Simple Go preprocessor, not a framework.
5. **URL**: Default GitHub Pages initially, custom domain deferred.
6. **Architecture decisions (solutions docs)**: Stay internal, not on public site.
7. **Versioning**: No — single version of docs. Users should be on latest.
