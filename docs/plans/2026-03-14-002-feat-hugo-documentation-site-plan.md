---
title: "feat: Hugo documentation site"
type: feat
status: active
date: 2026-03-14
origin: docs/brainstorms/2026-03-14-hugo-documentation-site-brainstorm.md
---

# feat: Hugo documentation site

## Overview

Build a Hugo-powered documentation site in `site/` that becomes the definitive reference for gh-velocity. Four content pillars: Getting Started, Guides (task-based), Concepts (explanations), and Reference (lookup). Deployed to GitHub Pages via GitHub Actions, with CLI reference and config schema auto-generated from Go code.

(See brainstorm: `docs/brainstorms/2026-03-14-hugo-documentation-site-brainstorm.md`)

## Problem Statement / Motivation

Documentation lives in a monolithic 56KB `docs/guide.md` plus a long README. Finding answers requires scrolling through a single file. There's no search, no sidebar navigation, and the structure mixes getting-started, conceptual, task-based, and reference content. The tool is mature enough to warrant professional docs that serve both individual developers and team leads.

## Proposed Solution

### Architecture

- **Hugo site** in `site/` using Hugo Book theme as a Hugo module
- **Four content pillars**: Getting Started, Guides, Concepts, Reference (the brainstorm had three; SpecFlow analysis identified that conceptual content like "Understanding the statistics" and "Why labels over project board" needs its own home)
- **Auto-generation**: `cmd/gendocs/main.go` runs cobra/doc for CLI reference pages and produces config schema from Go struct reflection
- **CI**: Separate `.github/workflows/docs.yml` deploys to GitHub Pages on push to main
- **Local dev**: `task docs:serve` for live preview

### Design Decisions Carried Forward from Brainstorm

1. Hugo Book theme — sidebar nav, search, professional layout (see brainstorm: key decision #2)
2. Decompose guide.md into individual pages (see brainstorm: key decision #4)
3. Auto-generate CLI reference via cobra/doc + config schema from Go structs (see brainstorm: key decision #5)
4. GitHub Pages, default URL initially (see brainstorm: key decision #6)
5. No versioning — single active version (see brainstorm: resolved question #7)
6. Architecture decisions (docs/solutions/) stay internal (see brainstorm: resolved question #6)

### Critical Clarifications from SpecFlow Analysis

- **Fourth pillar added**: `concepts/` for explanatory content (statistics, linking strategies, labels vs. project board, GitHub limitations). These don't fit getting-started, guides, or reference.
- **Config schema v1 is hand-written**: The preprocessor for config schema from Go struct reflection is complex (nested structs, defaults in code, validation logic separate from tags). v1 migrates the config reference from guide.md. v2 adds auto-generation.
- **Metric definitions are hand-written**: The brainstorm mentioned `// Definition:` tagged doc blocks — these don't exist yet. v1 hand-writes metric pages from guide.md content. v2 can add extraction if warranted.
- **cobra/doc front matter injection**: cobra/doc generates plain markdown without Hugo `title`/`weight` front matter. The preprocessor must post-process output to inject proper front matter.

## Technical Approach

### Site Structure

```
site/
├── hugo.toml
├── go.mod                              # Hugo module (imports Book theme)
├── go.sum
├── content/
│   ├── _index.md                       # Landing: value prop + links to pillars
│   ├── getting-started/
│   │   ├── _index.md                   # Section index (weight: 1)
│   │   ├── quick-start.md             # Install + first command (5 min)
│   │   ├── how-it-works.md            # Lifecycle diagram, signals, "what you need to do"
│   │   ├── configuration.md           # Preflight, config file, tokens
│   │   └── ci-setup.md                # GitHub Actions, token permissions, workflow YAML
│   ├── guides/
│   │   ├── _index.md                   # Section index (weight: 2)
│   │   ├── interpreting-results.md    # Reading output, what good looks like
│   │   ├── velocity-setup.md          # Effort strategies, iterations, field: matchers
│   │   ├── cycle-time-setup.md        # Strategies, lifecycle config, labels vs. board
│   │   ├── posting-reports.md         # --post, discussions, comments, CI posting
│   │   ├── agent-integration.md       # JSON output, jq, Claude Code/Copilot
│   │   ├── recipes.md                 # Compare releases, find slowest, export CSV
│   │   └── troubleshooting.md         # Error messages, N/A results, common issues
│   ├── concepts/
│   │   ├── _index.md                   # Section index (weight: 3)
│   │   ├── github-capabilities.md     # What GitHub can and cannot tell you
│   │   ├── statistics.md              # Median vs mean, P90/P95, IQR, outliers
│   │   ├── linking-strategies.md      # pr-link, commit-ref, changelog in depth
│   │   └── labels-vs-board.md         # Why labels for cycle time, what board is for
│   ├── reference/
│   │   ├── _index.md                   # Section index (weight: 4)
│   │   ├── metrics/
│   │   │   ├── _index.md              # Metrics overview
│   │   │   ├── lead-time.md           # Our definition, formula, signals
│   │   │   ├── cycle-time.md          # Our definition, strategies, signals
│   │   │   ├── velocity.md            # Effort, iterations, completion rate
│   │   │   ├── throughput.md          # Count-based flow
│   │   │   └── quality.md             # Defect rate, hotfix window, categories
│   │   ├── commands/                   # Auto-generated from cobra/doc
│   │   │   ├── _index.md
│   │   │   └── *.md                   # One per leaf command
│   │   ├── config.md                   # Hand-written v1, auto-generated v2
│   │   └── api-consumption.md         # Rate limits, budget, caching
│   └── examples/
│       ├── _index.md                   # (weight: 5)
│       └── configs.md                  # Annotated example configs (tabs or accordion)
├── static/
│   └── images/
└── layouts/                            # Theme overrides if needed
```

### Complete Migration Inventory

Every H2/H3 from guide.md mapped to a destination:

| guide.md Section | Destination Page | Notes |
|---|---|---|
| Why this exists | getting-started/_index.md | Opening paragraph |
| What the metrics mean | reference/metrics/_index.md | Overview definitions |
| How your GitHub workflow becomes metrics | getting-started/how-it-works.md | Lifecycle diagram, signals table |
| The lifecycle of an issue | getting-started/how-it-works.md | |
| Start and end signals | getting-started/how-it-works.md | |
| What you need to do | getting-started/how-it-works.md | Progressive disclosure |
| What the tool reads at each level | getting-started/how-it-works.md | |
| Choosing a cycle time strategy | guides/cycle-time-setup.md | |
| Configuring the issue strategy | guides/cycle-time-setup.md | |
| Configuring the PR strategy | guides/cycle-time-setup.md | |
| Solo developers vs. teams | getting-started/how-it-works.md | Workflow archetypes |
| Connecting PRs to issues | concepts/linking-strategies.md | |
| What GitHub can and cannot tell you | concepts/github-capabilities.md | Full section |
| Getting started (Prerequisites, Install, First queries) | getting-started/quick-start.md | |
| Your own repo / Cycle time works remotely | getting-started/configuration.md | |
| Configuration reference (Minimal, Full, Details) | reference/config.md | |
| Validating your config | reference/config.md | |
| Linking strategies in depth | concepts/linking-strategies.md | |
| Use with an agent | guides/agent-integration.md | |
| CI integration (all subsections) | getting-started/ci-setup.md | |
| How-to recipes (all) | guides/recipes.md | |
| Understanding the statistics | concepts/statistics.md | |
| Why labels over project board | concepts/labels-vs-board.md | |
| Troubleshooting (all) | guides/troubleshooting.md | |

### Implementation Phases

#### Phase 1: Hugo Scaffolding & Theme

Set up the Hugo site skeleton with Book theme, empty content pages, and local preview.

**Tasks:**

- [ ] Create `site/` directory structure
- [ ] Initialize Hugo module: `hugo mod init github.com/dvhthomas/gh-velocity` in `site/`
- [ ] Create `site/hugo.toml` with Book theme config, `BookSection = "docs"` pointing to content root, search enabled, code copy buttons enabled
- [ ] Create `site/go.mod` and `site/go.sum` (Hugo module for theme import)
- [ ] Create `site/content/_index.md` — landing page with value proposition and pillar links
- [ ] Create all `_index.md` section files with proper `weight` and `title` front matter
- [ ] Create stub pages for every destination in the migration inventory (title + "TODO: migrate from guide.md § section name")
- [ ] Add to Taskfile: `task docs:serve` → `hugo server --source site`
- [ ] Add to Taskfile: `task docs:build` → `hugo --source site --minify`
- [ ] Verify: `task docs:serve` renders sidebar with all sections, navigation works

**Files:**
- `site/hugo.toml` (new)
- `site/go.mod` (new)
- `site/content/**/_index.md` (new, ~12 files)
- `site/content/**/*.md` (new stubs, ~20 files)
- `Taskfile.yaml` (modify)

#### Phase 2: Content Migration — Getting Started

Migrate the most critical content first: what a new user needs.

**Tasks:**

- [ ] Migrate `quick-start.md` from guide.md § "Getting started" (Prerequisites, Install, First queries)
- [ ] Write `how-it-works.md` from guide.md § "How your GitHub workflow becomes metrics" — lifecycle diagram (convert ASCII to Mermaid), signals table, "what you need to do", solo vs. teams
- [ ] Migrate `configuration.md` from guide.md § "Your own repo", "Configuration reference" (overview only, not full schema), token permissions
- [ ] Migrate `ci-setup.md` from guide.md § "CI integration" (all subsections), include the repo's own `velocity.yaml` as a real-world example
- [ ] Cross-link between pages (e.g., quick-start → how-it-works → configuration → ci-setup)
- [ ] Review: read through as a new user, verify the flow makes sense

**Files:**
- `site/content/getting-started/*.md` (4 files)

#### Phase 3: Content Migration — Guides

Task-oriented content for users who know the tool and need to accomplish something specific.

**Tasks:**

- [ ] Migrate `interpreting-results.md` — reading pretty/JSON/markdown output, what "good" looks like, common patterns
- [ ] Migrate `velocity-setup.md` — effort strategies (count, attribute, numeric, field:), iteration strategies, preflight suggestions
- [ ] Migrate `cycle-time-setup.md` — strategy choice, lifecycle config, absorb key points from "Why labels over project board" (with link to concepts page)
- [ ] Migrate `posting-reports.md` from guide.md § "Posting to GitHub", discussions config
- [ ] Write `agent-integration.md` from guide.md § "Use with an agent"
- [ ] Migrate `recipes.md` from guide.md § "How-to recipes" (all subsections)
- [ ] Migrate `troubleshooting.md` from guide.md § "Troubleshooting" (all error messages and symptoms)

**Files:**
- `site/content/guides/*.md` (7 files)

#### Phase 4: Content Migration — Concepts

Explanatory content that helps users understand *why* things work the way they do.

**Tasks:**

- [ ] Migrate `github-capabilities.md` from guide.md § "What GitHub can and cannot tell you"
- [ ] Migrate `statistics.md` from guide.md § "Understanding the statistics"
- [ ] Migrate `linking-strategies.md` from guide.md § "Linking strategies in depth" + "Connecting PRs to issues"
- [ ] Migrate `labels-vs-board.md` from guide.md § "Why labels over project board for cycle time"

**Files:**
- `site/content/concepts/*.md` (4 files)

#### Phase 5: Content Migration — Reference

Lookup-oriented content for users who know what they're looking for.

**Tasks:**

- [ ] Write metric reference pages (`lead-time.md`, `cycle-time.md`, `velocity.md`, `throughput.md`, `quality.md`) — our specific definitions, formulas, start/end signals, config that affects them
- [ ] Hand-write `config.md` — full config schema reference migrated from guide.md § "Configuration reference", organized by section (scope, project, lifecycle, quality, velocity, etc.)
- [ ] Write `api-consumption.md` — per-command cost estimates, rate limit budget, caching behavior
- [ ] Write `examples/configs.md` — annotated example configs using Hugo Book tabs or collapsible sections

**Files:**
- `site/content/reference/**/*.md` (~10 files)
- `site/content/examples/*.md` (1 file)

#### Phase 6: CLI Reference Auto-Generation (cobra/doc)

Auto-generate command reference pages from Cobra metadata.

**Tasks:**

- [ ] Create `cmd/gendocs/main.go`:
  - Import the root Cobra command
  - Use `doc.GenMarkdownTreeCustom` with a `filePrepender` that injects Hugo front matter (`title`, `weight`, `bookToC: true`)
  - Use a `linkHandler` that rewrites `.md` links for Hugo's URL scheme
  - Transform cobra/doc file names to Hugo-friendly format
  - Output to `site/content/reference/commands/`
- [ ] Add to Taskfile: `task docs:generate` → `go run ./cmd/gendocs`
- [ ] Update `task docs:build` to depend on `task docs:generate`
- [ ] Verify: all 14 leaf commands get reference pages with correct titles and sidebar ordering
- [ ] Verify: command examples render properly in the Hugo theme

**Files:**
- `cmd/gendocs/main.go` (new)
- `Taskfile.yaml` (modify)

#### Phase 7: GitHub Actions Deployment

Automate docs build and deploy on push to main.

**Tasks:**

- [ ] Create `.github/workflows/docs.yml`:
  - Trigger: push to main (not PRs — avoid blocking development)
  - Install Hugo Extended (pin version, e.g., 0.147.0)
  - Setup Go (for Hugo modules + gendocs)
  - Run `task docs:generate`
  - Run `hugo --source site --minify --baseURL` with `${{ steps.pages.outputs.base_url }}`
  - Upload artifact via `actions/upload-pages-artifact@v3`
  - Deploy via `actions/deploy-pages@v4`
  - Permissions: `contents: read`, `pages: write`, `id-token: write`
- [ ] Enable GitHub Pages in repo settings: Source → GitHub Actions
- [ ] Verify: push to main triggers build and deploy
- [ ] Verify: site is accessible at `dvhthomas.github.io/gh-velocity`

**Files:**
- `.github/workflows/docs.yml` (new)

#### Phase 8: Deprecate guide.md & Update README

Clean up the old docs and point to the new site.

**Tasks:**

- [ ] Replace `docs/guide.md` content with a redirect notice: "This guide has moved to [site URL]. The content below is no longer maintained."
- [ ] Keep the first paragraph as a summary for anyone who lands on the file via old links
- [ ] Update README.md: replace `docs/guide.md` links with site URLs
- [ ] Update README.md: shrink the inline config reference to a summary + link to site
- [ ] Update AGENTS.md if it references guide.md
- [ ] Search repo for other links to guide.md sections and update them
- [ ] Verify: no broken internal links

**Files:**
- `docs/guide.md` (modify — shrink to redirect)
- `README.md` (modify)
- `AGENTS.md` (modify if needed)

## Acceptance Criteria

### Functional Requirements

- [ ] `task docs:serve` starts local Hugo server with sidebar navigation
- [ ] `task docs:build` produces a complete static site in `site/public/`
- [ ] `task docs:generate` produces CLI reference pages from Cobra commands
- [ ] All guide.md content is accounted for in the migration inventory — no content lost
- [ ] Site has four pillars: Getting Started, Guides, Concepts, Reference
- [ ] Every metric has a dedicated reference page with our specific definition
- [ ] Command reference pages are auto-generated with proper titles and ordering
- [ ] Config reference page documents all fields with types, defaults, and constraints
- [ ] GitHub Action deploys on push to main
- [ ] Site is accessible at GitHub Pages URL

### Non-Functional Requirements

- [ ] Hugo Extended version pinned in CI and documented in Taskfile
- [ ] Search works across all content (Hugo Book built-in)
- [ ] Code blocks have copy buttons
- [ ] Site renders well on mobile (Hugo Book responsive)
- [ ] Build completes in under 30 seconds

### Quality Gates

- [ ] New user can go from landing page to running first command following only site content
- [ ] Every error message in Troubleshooting is searchable on the site
- [ ] No broken internal cross-links between pages
- [ ] `docs/guide.md` replaced with redirect notice pointing to site

## Dependencies & Risks

- **Hugo version**: Hugo has breaking changes between minor versions. Pin version in CI and Taskfile. Hugo Book theme may lag behind Hugo releases.
- **Theme maintenance**: hugo-book is maintained by a single author. If abandoned, the site still works — just no new features. Hugo's module system makes theme switching straightforward.
- **Config schema auto-generation**: Deferred to v2. The preprocessor for nested Go structs with defaults and validation is complex. Hand-written v1 is safer.
- **Content freshness**: Hand-written guides can drift from code. Consider adding a `docs-freshness` smoke test that greps site content for command/flag names and verifies they exist in the binary.

## Future Considerations

- **Config schema auto-generation (v2)**: Walk Go structs via `go/ast` for doc comments + reflection for types/tags. Emit markdown with nested sections. Include defaults from `defaults()` function and validation constraints.
- **Metric definition extraction (v2)**: Add `// Definition:` tagged doc blocks to metric computation functions. Preprocessor extracts them into reference pages.
- **Custom domain**: Add CNAME for `velocity.bitsby.me` when ready.
- **Versioned docs**: If breaking config changes warrant it. Hugo Book supports version dropdown.
- **Screenshots/diagrams**: Mermaid for lifecycle flow, actual CLI output screenshots for guides.

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-14-hugo-documentation-site-brainstorm.md](../brainstorms/2026-03-14-hugo-documentation-site-brainstorm.md) — key decisions: Hugo Book theme, decompose guide.md, auto-generate CLI+config, GitHub Pages, no versioning.

### Internal References

- Existing guide: `docs/guide.md` (56KB, ~1150 lines, 9 H2 sections)
- README: `README.md`
- Example configs: `docs/examples/*.yml`
- Solutions docs: `docs/solutions/*.md` (stay internal)
- Cobra commands: `cmd/*.go` (Short/Long/Example fields)
- Config structs: `internal/config/config.go`
- CI workflows: `.github/workflows/*.yml`

### External References

- Hugo Book theme: https://github.com/alex-shpak/hugo-book
- Hugo documentation: https://gohugo.io/documentation/
- cobra/doc API: https://pkg.go.dev/github.com/spf13/cobra/doc
- GitHub Pages deployment: https://gohugo.io/hosting-and-deployment/hosting-on-github/
- Blog post pattern: https://bitsby.me/til/2021-02-28/how-to-make-a-hugo-blog-from-scratch/
