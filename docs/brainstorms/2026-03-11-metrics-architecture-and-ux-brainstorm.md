---
title: Metrics Architecture and UX Improvements
date: 2026-03-11
status: draft
origin: conversation during my-week implementation (PR #43)
---

# Metrics Architecture and UX Improvements

Three ideas that emerged while building `my-week`. Each is independent but they reinforce each other.

---

## 1. Config Required — No Implicit Fallback

### Problem

The implicit `repo:owner/repo` fallback when no config exists causes repeated confusion. The `-R` flag, config scope, and fallback scope interact in surprising ways. We kept tripping over "which scope am I actually using?" bugs during my-week development.

### Decision

**All commands except `preflight` require `.gh-velocity.yml`.** No implicit fallback.

- `preflight` is the entry point — it generates config
- Commands without config fail with a clear message: `"No config found. Run 'gh velocity preflight' to get started."`
- `-R` flag determines which repo context to use for finding/generating config
- Config is where scope, categories, strategies, and all rich settings live
- CLI flags should NOT mirror every config value — config exists because these settings are too rich for flags

### Implementation

- Add config check in `root.go` where `Deps` is constructed
- Remove implicit `repo:owner/repo` fallback from scope resolution
- Ensure `preflight` docs and error messages are excellent
- Document every config setting with examples of common combos (single repo, cross-repo, project board)

### Documentation Requirements

Every setting needs:
- What it does
- Default value (if any)
- Common combinations (single repo, cross-repo, project board)
- Example snippets showing real configs

---

## 2. Empty Block Messaging with Evidence Links

### Problem

When a section in `my-week` (or any command) is empty, we show `_None_` or skip it. This doesn't help the user understand *why* — is it a scope problem? No data? Wrong time window? They can't verify.

### Proposal

Empty sections should explain why and link to the GitHub search query so users can verify independently.

**Pretty output:**
```
Issues Closed: 0
  No issues closed by @dvhthomas in this period.
  Verify: https://github.com/search?q=repo:dvhthomas/gh-velocity+is:issue+is:closed+author:dvhthomas+closed:2026-03-01..2026-03-08
```

**Markdown output:**
```markdown
**Issues Closed (0)**

_No issues closed by @dvhthomas in this period. [Verify search](https://github.com/search?q=...)_
```

**JSON output:**
```json
{
  "issues_closed": [],
  "issues_closed_search_url": "https://github.com/search?q=..."
}
```

### Scope

Apply to all commands, not just my-week. Every empty result should include:
1. A human-readable explanation of what was searched
2. A clickable link to the GitHub search (so users can see if the query is wrong or the data genuinely doesn't exist)

### Implementation

- The `scope.Query` already has `Build()` which returns the search string
- Add a `SearchURL()` method that returns the GitHub search web URL for the same query
- Pass search URLs through to formatters alongside results
- Formatters render the explanation + link when results are empty

---

## 3. Composable Metrics Interface

### Problem

Lead time, cycle time, and review time are currently computed in different places with different calling conventions. `my-week` computes them ad-hoc in `ComputeInsights`. The `leadtime` command computes them differently. There's no way to reuse "lead time for a person" vs "lead time for all people" vs "lead time for a single issue".

### Proposal

Each metric has three natural scopes:

```
ForItem(issue/PR)        → single result (e.g., "this issue took 3 days")
ForPerson(login, scope)  → []result for one person (e.g., "your median lead time")
ForScope(scope)          → []result for all people minus bots (e.g., "team lead time")
```

The existing code already has these but they're scattered:
- `metrics.LeadTime(issue)` = ForItem
- `ComputeInsights` computes ForPerson ad-hoc
- `cmd/leadtime.go` computes ForScope ad-hoc

### Proposed Interface

```go
// internal/metrics/metric.go

type MetricResult struct {
    Items []model.Metric  // individual measurements
    Stats model.Stats     // aggregate statistics
}

type ScopedMetric interface {
    // ForItem computes the metric for a single issue or PR.
    ForItem(item interface{}) model.Metric

    // ForPerson computes the metric for items attributed to a specific person.
    // Uses the same search queries as ForScope but adds author/assignee filters.
    ForPerson(items []interface{}) MetricResult

    // ForScope computes the metric for all items in scope.
    ForScope(items []interface{}) MetricResult
}
```

### How It Fits

- `my-week` calls `LeadTime.ForPerson(closedIssues)` and `CycleTime.ForPerson(mergedPRs)`
- `flow lead-time` calls `LeadTime.ForScope(closedIssues)` or `LeadTime.ForItem(issue)`
- `flow cycle-time` calls `CycleTime.ForScope(mergedPRs)` or `CycleTime.ForItem(pr)`
- Future `status reviews` could use `ReviewTime.ForPerson(reviewedPRs)`

### Key Constraint

The interface operates on **already-fetched data** (slices of model types). It does NOT fetch data itself. Data fetching stays in commands; metric computation stays pure.

This means `ForPerson` and `ForScope` take the same input type — the difference is in *what data the command fetched* (filtered by author vs not), not in the metric computation.

Simpler version: maybe just `Compute(items) → MetricResult` where commands are responsible for fetching the right subset. The ForItem/ForPerson/ForScope distinction is in the *command*, not the metric.

### Open Questions

- Is the interface worth the abstraction, or is "each command calls `metrics.ComputeStats(durations)`" simple enough?
- Should metrics know about their display names, units, and formatting? Or keep that in formatters?
- How does this interact with cycle time strategies (issue vs PR vs project-board)?

---

## Priority Assessment

| Idea | Value | Effort | When |
|------|-------|--------|------|
| Config required | High — eliminates a class of bugs | Low — single check in root.go | Next (before more features) |
| Empty block messaging | Medium — better UX, self-service debugging | Medium — need SearchURL on Query, update all formatters | After config-required |
| Metrics interface | Medium — code reuse, consistency | High — touches metrics, model, all commands | After phases 2b-4, when patterns are clearer |
