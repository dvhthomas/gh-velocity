---
title: "gh-velocity: GitHub Velocity & Quality Metrics"
date: 2026-03-09
status: completed
type: brainstorm
---

# gh-velocity: GitHub Velocity & Quality Metrics

## What We're Building

A GitHub CLI extension (`gh velocity`) that calculates and reports development velocity and quality metrics for GitHub repositories using established, well-understood metrics (DORA, etc.). It posts formatted metrics to issues, discussions, and release notes — making engineering performance visible where the work happens.

This is a Go rewrite and evolution of the bash metrics scripts in [go-calcmark](https://github.com/bitsbyme/go-calcmark), informed by the [Compound Engineering](https://bitsby.me/2026/03/compound-engineering/) methodology.

## Why This Approach

Commercial dashboard tools exist (LinearB, Jellyfish, Sleuth, etc.) but they're heavyweight, expensive, and opaque. gh-velocity is:

- **Composable** — Each metric is a subcommand. Pipe, combine, script as needed.
- **Agent-friendly** — JSON output lets AI agents consume and reason about metrics.
- **In-context** — Metrics posted directly on issues, discussions, and release notes where they're actionable.
- **Transparent** — Open source, auditable calculations, configurable definitions.
- **Workflow-agnostic** — Works for solo devs (commit-based) and teams (PR-based). Core metrics don't require PRs.

## Metric Philosophy

Two guiding principles:

1. **No invented metrics.** Every metric maps to an established, well-understood industry metric (DORA, SPACE, etc.). We provide a way to compute them from GitHub data.

2. **Outcome-based quality.** Quality metrics measure what happened after you shipped (escaped defects, recovery time, change failure rate), not how you reviewed before shipping. This makes quality measurable regardless of whether the repo uses PRs. When PRs are present, they provide optional enrichment (review time, size metrics) — but the core quality story is told through outcomes.

This is especially important in an AI-writes-code world: humans own quality regardless of who wrote the code. The metrics should answer "are we shipping reliable software?" not "did we follow a specific process?"

## Key Decisions

### Distribution: gh CLI Extension
- Installed via `gh extension install`
- Leverages gh's authentication and repo context
- Feels native to GitHub workflows

### Architecture: Command-per-metric
- Separate subcommands per metric category
- Shared `--format` flag: `json` | `csv` | `pretty` | `markdown`
- `--post` flag to write to GitHub (default: stdout only)
- Mirrors existing bash script structure for easy migration

### Workflow Mode: Config Flag with Auto-detect
- `.gh-velocity.yml` can declare `workflow: local` or `workflow: pr`
- If unset, auto-detect from repo patterns (has merged PRs? → pr mode)
- In `local` mode: cycle time uses commits. In `pr` mode: cycle time uses PR lifecycle.

### Velocity Metrics
- **Lead Time:** Issue created → appears in release tag (or now if unreleased). Maps to DORA Change Lead Time adapted for issue-level tracking.
- **Cycle Time:** First commit → last commit for an issue (local mode) or PR created → PR merged (pr mode).
- **Throughput:** Issues completed per period, PRs merged per period.
- **Plan vs. Actual:** Compare GitHub Projects' Start Date / Target Date fields against actual work start (first commit) and completion (Done/Closed). Measures estimation accuracy.

### Quality Metrics (Outcome-based, no PRs required)
- **Change Failure Rate** (DORA): % of releases requiring a hotfix. Derived from release tags with convention (e.g., patch release following a minor/major = hotfix).
- **Recovery Time** (DORA MTTR): Time from bug issue filed to fix release shipped.
- **Escaped Defects:** Bug-labeled issues created after a release, counted per release.
- **Rework Rate:** Issues reopened or requiring follow-up fixes. Maps to DORA's rework rate concept (2024+).

### PR Metrics (Optional Enrichment)
Available when the repo uses PRs. Not required for core velocity or quality reporting.
- **PR Size:** Additions, deletions, changed files, commit count.
- **PR Review Time:** PR created → first review, first review → merge.
- **Review Depth:** Comments per PR review.

### Configuration: `.gh-velocity.yml` in repo
- Per-repo config checked into version control
- Teams share field mappings
- Agents can read and reason about the config

Example structure:
```yaml
workflow: local  # or "pr" — auto-detected if omitted

project:
  name: "My Project"
  id: "PVT_xxx"
  status_field_id: "PVTSSF_xxx"

statuses:
  backlog: "Backlog"        # Lead time clock starts
  ready: "Ready"
  in_progress: "In progress"
  in_review: "In review"
  done: "Done"              # Clocks stop

fields:
  start_date: "Start Date"  # Planned start (optional Projects field)
  target_date: "Target Date" # Planned completion (optional Projects field)

quality:
  bug_labels: ["bug", "defect"]
  hotfix_window_hours: 72  # Patch release within this window after minor/major = hotfix

discussions:
  category_id: "DIC_xxx"    # For release velocity posts
```

### GitHub Projects: v2 Only (for v1)
- Status transitions from project board fields
- Simpler implementation, matches CalcMark setup
- Can add label/milestone fallback in a future version

### Issue Grouping: Git Tags + Commit Messages
- Find issues referenced in commits between two tags
- Same proven pattern as existing release-velocity.sh
- Support explicit `--issues 1,2,3` override for ad-hoc grouping

### Project-level Reporting
- `gh velocity project` reports on all issues in a GitHub Projects v2 board
- `--from` / `--to` date flags constrain which issues to include (by close date or status transition date)
- Enables tracking change over time: run for Q1 vs Q2, or month-over-month
- Aggregate stats (mean, median, SD) across the filtered issue set
- Same output formats as other commands (JSON, CSV, pretty, markdown)

### Posting: Generate + Post with Flag
- Default: output to stdout (safe for scripting and preview)
- `--post` flag: write to GitHub (issue comment, discussion, release notes)
- Enables dry-run workflows and agent-driven posting

### Output Formats
- **JSON** — Agent-readable, structured data
- **CSV** — Spreadsheet-friendly, data analysis
- **Pretty** — Console output with aligned columns and duration formatting
- **Markdown** — For insertion into discussions, release notes, issue comments

### GitHub API Strategy: go-github Library
- Type-safe, mockable interfaces for excellent test coverage
- Auth delegates to gh's token (`gh auth token`)
- Enables thorough unit testing without subprocess mocking

### Commit-to-Issue Linking: Hardcoded Defaults
- Ship with proven heuristics: `(#N)` in commit message, PR references, issue body mentions
- Same patterns as existing bash scripts — covers 95% of cases
- Add configurable patterns later if needed

### Language & Testing
- Written exclusively in Go
- Excellent test coverage with table-driven tests

## Subcommands (v1)

| Command | Scope | Description |
|---------|-------|-------------|
| `gh velocity lead-time <issue>` | Single issue | Issue created → release/now |
| `gh velocity cycle-time <issue>` | Single issue | First commit → last commit (local) or PR lifecycle (pr mode) |
| `gh velocity summary <issue>` | Single issue | All applicable metrics: lead time, cycle time, rework, plan-vs-actual (when Projects fields exist) |
| `gh velocity pr-metrics <pr>` | Single PR | Size, review time, depth (when PRs are used) |
| `gh velocity release <tag>` | Release | Aggregate velocity + quality for all issues in release, with mean/median/SD |
| `gh velocity throughput` | Repo | Issues closed + PRs merged per period |
| `gh velocity project` | Project | Velocity + quality metrics across a project's issues, with `--from`/`--to` date filtering |

## Output Format Reference

Based on [CalcMark Release Velocity Discussion #42](https://github.com/CalcMark/go-calcmark/discussions/42), the markdown output for a release should look like:

```markdown
## Release Velocity: v1.6.5

**Released:** 2026-03-09
**Commits:** 13 (since v1.6.4)
**Issues closed:** 3

### Metrics

| Issue | Title | Lead Time | Cycle Time | Commits |
|-------|-------|-----------|------------|---------|
| #32 | Add cross-layer consistency checks | 3d 13h | 10h 43m | 2 |
| #40 | fix: compound() ignores bare period modifiers | 1h 27m | 28m | 8 |
| #41 | fix: NL function eval errors show wrong line | 4m | 5m | 2 |
| | **Mean** | **1d 5h** | **3h 45m** | **4.0** |
| | **Median** | **1h 27m** | **28m** | **2.0** |
| | **Std Dev** | **1d 14h** | **5h 58m** | **3.5** |
```

### Aggregate Statistics
All multi-issue views (release, throughput, grouped reports) include summary statistics:
- **Mean** — Average across issues in the set
- **Median** — Middle value, more robust to outliers than mean
- **Standard Deviation** — Spread/consistency indicator

These aggregates apply to lead time, cycle time, and any numeric metric column.

## Resolved Questions

1. **GitHub API strategy:** go-github library. Type-safe, mockable, aligns with excellent test coverage goal. Auth delegates to gh's token.

2. **Test coverage delta tracking:** Deferred to v2. Most complex quality metric, requires CI integration. Ship v1 with outcome-based quality metrics that work from GitHub data alone.

3. **Commit-to-issue linking:** Hardcoded defaults using proven heuristics from existing scripts. Configurable patterns deferred to later if needed.

4. **Metric philosophy:** Use established metrics only (DORA, etc.). Quality metrics are outcome-based and don't require PRs. PR metrics are optional enrichment.

5. **gh extension bootstrapping:** Use `gh extension create --precompiled=go` for official scaffolding. Ensures correct binary naming, build tags, and install compatibility.

6. **Hotfix detection:** Semver convention. A patch release (x.y.Z) within a configurable time window after a minor/major release is treated as a hotfix. Simple, works with semver repos, configurable window in `.gh-velocity.yml`.

7. **DORA metrics as future feature:** Full DORA metric support (Deployment Frequency, Lead Time for Changes, Change Failure Rate, MTTR) deferred to a future version. DORA metrics are quite specific — they require GitHub Actions deployment event data and incident tracking (e.g., PagerDuty, Datadog) beyond what v1's issue/commit/release model provides. Our v1 metrics are *informed by* DORA concepts but are not strict DORA implementations. See [github-dora-metrics](https://github.com/mikaelvesavuori/github-dora-metrics) and [lead-time-for-changes](https://github.com/DeveloperMetrics/lead-time-for-changes) for reference implementations that could inform the future DORA module.
