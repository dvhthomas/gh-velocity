---
title: "Actionable Output — Making gh-velocity Indispensable"
date: 2026-03-10
status: active
type: brainstorm
---

# Brainstorm: Actionable Output — Making gh-velocity Indispensable
**Scope:** Product vision for transforming gh-velocity from "shows metrics" to "drives action" across developers, PMs, and engineering leaders

## The Problem

gh-velocity currently shows metrics. A developer sees "median lead time 5d, 2 outliers" and thinks "ok, cool" — then does nothing. The output is informational but not actionable. The tool is used occasionally, not daily.

**The gap isn't missing features — it's missing answers to the questions people actually ask:**

| Persona | What they ask | What gh-velocity answers today |
|---------|--------------|-------------------------------|
| Developer | "What should I update my manager on?" | Nothing — they write status updates manually |
| Developer | "Why does everything feel slow?" | "Cycle time is 5 days" — but not WHERE time is lost |
| Tech Lead | "Who's drowning in reviews?" | Nothing — review load is invisible |
| PM | "Did scope creep cause the slip?" | Nothing — no planned-vs-shipped comparison |
| Eng Leader | "Are we getting better or worse?" | A single snapshot — no trends |
| Eng Leader | "What's our bus factor risk?" | Nothing — knowledge concentration unmeasured |

## Research Foundations

This brainstorm is grounded in external research, not assumptions:

- **Trust drives adoption**: Swarmia won developer hearts by making every metric "view source" — click through to the underlying items. Opaque aggregates get abandoned. (Source: G2 reviews, Swarmia vs LinearB evaluations)
- **The AI review bottleneck is real**: AI tools increase PR output ~98% but review wait time increases ~91%. AI-generated PRs wait 4.6x longer for review. No CLI tool surfaces this. (Source: 2025 DORA Report, byteiota.com research)
- **Flow efficiency is the most underused metric**: Most organizations have <20% flow efficiency — work spends 80% of time waiting. Almost no developer-facing tool surfaces this breakdown. (Source: Planview, Minware)
- **Developers want self-serve data**: They want to advocate for themselves in 1:1s, identify bottlenecks, and argue for process improvements with evidence. They don't want surveillance. (Source: HN/Reddit developer communities, McKinsey backlash)
- **CLI stickiness comes from daily rituals**: gh-dash succeeded by being the thing developers open every morning. Speed, composability, and "no context switch" keep developers in the terminal. (Source: gh-dash adoption patterns)

## The Vision: Eight Features That Drive Daily Use

### 1. My Week — The Developer's 1:1 Prep Tool

`gh velocity my-week` / `gh velocity my-week --format markdown`

A personal activity summary designed to be pasted into a 1:1 doc or Slack:
- Issues closed this week (with links)
- PRs merged (with links)
- PRs reviewed (count + links)
- Review turnaround time (your average)
- Items still in progress (with age)

**Why it's sticky:** Developers hate writing status updates but need them weekly. This writes it in 5 seconds. Makes the tool valuable to the *individual*, not just the team.

**Persona boundary:** Self-serve only. Shows YOUR activity (via `gh auth status` identity). No `--user` flag to look at others. Team-level data is always aggregated, never per-person.

**Data source:** GitHub search API filtered by author/reviewer + authenticated user. Minimal new API surface.

### 2. Wait-State Decomposition — "Where Did the Time Go?"

Enhance cycle time output to break duration into segments:

```
#142  "Add export feature"    Cycle: 6d 4h
      Coding:         8h  ████░░░░░░░░░░░   5%
      Waiting review: 3d  ████████████░░░░  50%
      In review:      4h  █░░░░░░░░░░░░░░░   3%
      Waiting merge: 2d 4h ████████░░░░░░░░  34%
      Blocked:       12h  ██░░░░░░░░░░░░░░   8%
```

**Why it's sticky:** Proves what developers *feel* — "I'm fast, the process is slow." Gives evidence to argue for process improvements. A manager sees "50% of time is waiting for review" and acts on it, rather than just seeing "cycle time is 6 days."

**Data source:** PR timeline events (opened → review requested → first review → approved → merged). Project board status changes (StatusAt) for the coding/blocked segments. Needs new API calls for PR timeline events.

**Works in GitHub Action too:** Timeline events are API-only, so this runs the same locally and in CI.

### 3. Review Pressure — The AI-Era Bottleneck Detector

`gh velocity status reviews`

Surfaces the invisible work of code review:
- PRs currently awaiting review (sorted by age, with links)
- Team review load this period (aggregated — "N reviews total, median turnaround Xh")
- Review-to-merge ratio (how many reviews before merge? high = churn signal)
- Stale review requests (>48h without response)

**Why it's sticky:** With AI generating more PRs, review is the new bottleneck. This is the first CLI tool to surface it. The stale-reviews list alone would be run daily by tech leads.

**Persona boundary:** Shows team aggregates for load distribution, never individual rankings. The "PRs awaiting review" list is valuable without attribution — it's about the *work*, not the *person*.

**Data source:** GitHub PR review API, PR timeline events. New API surface but high value.

### 4. Bus Factor / Knowledge Risk

`gh velocity quality bus-factor`

Uses local git history to compute:
- Files/directories with only 1 contributor in the last 90 days
- Knowledge concentration score per directory (higher = riskier)
- Top risk areas with their sole contributors

```
Risk   Path                    Contributors (90d)   Primary
HIGH   internal/strategy/      1                    alice (100%)
MEDIUM internal/github/        2                    bob (78%), carol (22%)
LOW    internal/metrics/       4                    distributed
```

**Why it's sticky:** Every team worries about this but nobody measures it. Pure local git analysis = instant, offline, zero API calls. Engineering leaders ask this question constantly but guess the answer.

**GitHub Action angle:** This is a perfect candidate for periodic CI runs — "weekly bus factor report posted to a discussion."

**Data source:** `git log --format='%an' --since='90 days ago' -- <path>`. Entirely local. The `internal/git/` package can handle this already.

### 5. Stale Work Detector — "What's Rotting?"

Enhance `status wip` with staleness signals:

```
#  Title                    Status        Age    Last Activity    Signal
87 Payment refactor         In Progress   23d    18d ago          🔴 STALE
52 API v2 migration         In Review     12d    11d ago          🔴 STALE
93 Add dark mode            In Progress    8d    2d ago           🟡 AGING
71 Fix login flow           In Progress    3d    1d ago           🟢 ACTIVE
```

Staleness signals:
- Time since last commit on associated branch
- Time since last comment on issue/PR
- Time since last project board status change
- Items in progress for >2x team median cycle time → flagged

**Why it's sticky:** Stale work is invisible until it's a crisis. This makes it visible during standup. The "last activity" column answers "is anyone actually working on this?"

**Data source:** Mostly existing (`StatusAt` on project items). May need `updated_at` on issues. Branch activity needs git log or API.

### 6. Flow Efficiency Score

Add a single metric to the report command:

```
Flow Efficiency: 18%  (active work: 1.2d / total elapsed: 6.5d)
                       ↑ This means 82% of time is waiting
```

One number. Massive impact. Most teams are shocked to learn they're at 15-20%. This drives more process improvement conversations than any other metric.

**Why it's sticky:** It's the "credit score for your engineering process." Simple to understand, hard to ignore, motivating to improve.

**Data source:** Derived from wait-state decomposition (Idea 2). Once you have the segments, efficiency is just active-time / total-time.

### 7. Scope Creep Detector

For releases, compare what was planned vs. what shipped:

```
Release v2.3.0 — Scope Analysis
  Planned:    12 items (at tag v2.2.0)
  Shipped:    15 items
  Added:       5 items after release started
  Deferred:    2 items (still open)
  Scope change: +25%
```

**Why it's sticky:** PMs love this. Explains WHY releases slip without blaming anyone. "We planned 12 items, scope grew 25%" is a conversation starter, not an accusation.

**Data source:** Compare issue close dates against release window. Issues closed *after* the previous release tag but *before* the current one were "planned." Issues added to the milestone or linked to PRs merged late are "scope additions." Mostly computable from existing data.

### 8. Trend Arrows — "Getting Better or Worse?"

Compare every metric to the previous equivalent period:

```
Report: owner/repo (2026-02-08 → 2026-03-10)

  Lead Time:    median 3d 5h  ↓23% vs prev period     improving
  Cycle Time:   median 1d 8h  ↑12% vs prev period     ⚠ regressing
  Throughput:   42 closed     ↑40% vs prev period      improving
  Flow Eff:     18%           ↑3pts vs prev period      improving
  WIP:          7 items       →  same as prev period
  Defect Rate:  7%            ↓2pts vs prev period      improving
```

**Why it's sticky:** A number is informative. A number with a trend is *motivating*. Teams celebrate improvements and investigate regressions. This is the difference between a report you glance at and one you discuss in retros.

**Data source:** Run the same queries for the previous period (double the API calls, but cached results could mitigate). No new data types needed — just comparative computation.

## Foundation: Actionable Output Infrastructure

All 8 features depend on a shared output infrastructure upgrade:

### Per-Item Links
- Pretty format: URLs cmd-clickable in modern terminals; OSC 8 hyperlinks when TTY detected (polish, not blocking)
- Markdown format: `[#42](https://github.com/owner/repo/issues/42)` — full URLs always
- JSON format: `url` field on all item types

### Smart Sorting
- `--sort` flag on bulk commands: `age`, `updated`, `duration`, `closed`
- Default sort by actionability (duration desc for metrics, age desc for WIP)
- Each command exposes only sorts that make sense for its data

### Contextual Search URLs
- Always shown at bottom of output (no flag needed)
- GitHub search URLs with `is:`, `label:`, `closed:`, `updated:` qualifiers
- All three formats: markdown links, pretty URLs, JSON `search_urls` array

### Labels in Output
- Show in item-listing commands, not in aggregate views
- Help users spot patterns (e.g., all outliers have `blocked` label)

## Output & Formatting Constraints

**All features must support all three output formats:** pretty (terminal), markdown, and JSON.

**Pretty format:** Rich terminal output is OK — colors, bars, borders via libraries like lipgloss or similar. The visual bar charts in wait-state decomposition (Idea 2) and staleness signals (Idea 5) are examples. **No interactive TUI** — everything is static, printed output. The tool prints and exits.

**Markdown format:** Must be self-contained and useful when posted to GitHub (comments, discussions, releases via `--post`). Full URL links, not shorthand references.

**JSON format:** Machine-readable for downstream tools, dashboards, and CI pipelines. Include URLs, search links, and all computed data.

## Execution Context

**Local vs. API vs. GitHub Action:**
- Ideas 4 (Bus Factor) works best with local git — ideal for dev machines or GitHub Action with checkout
- Ideas 1, 2, 3 (My Week, Wait States, Reviews) are API-heavy — work anywhere, same local or CI
- Ideas 5, 7, 8 (Stale, Scope Creep, Trends) use existing API surface with minor additions
- Idea 6 (Flow Efficiency) is derived from Idea 2's data

**Persona boundary:** Self-serve for personal data. Team aggregates for shared metrics. Never individual rankings or comparisons. The tool serves developers first — they run it, they own it.

**Anti-patterns explicitly avoided:**
- No individual developer rankings or leaderboards
- No "developer productivity score" or single-number performance rating
- No lines-of-code or commit-count as standalone metrics
- Metrics are for learning and improvement, not evaluation

## Resolved Questions

1. **Trend period alignment:** Use named periods — compare this week/month/quarter to the previous one. More natural than raw day counts. Period names: `week`, `month`, `quarter`.

2. **My Week identity:** Strictly self-serve. Uses `gh auth status` identity. No `--author` flag. Team leads use `report` for team-level data. If demand emerges, revisit later.

3. **Wait-state accuracy:** Use "first commit timestamp → PR opened" as proxy for coding time. Not perfect, but practical. Decomposition focuses on PR lifecycle: coding → waiting review → in review → waiting merge → merged.

4. **Scope creep baseline:** Deferred. The concept is in the vision but the "planned vs. unplanned" mechanism needs more thought. Don't design it until there's a clear signal on whether teams use milestones, project boards, or something else to define release scope.
