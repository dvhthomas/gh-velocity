---
title: Linking Strategies & Flexible Classification
date: 2026-03-09
status: completed
type: brainstorm
---

# Linking Strategies & Flexible Classification

## What We're Building

A strategy-based system for discovering which issues and PRs belong to a release, replacing the current single-approach commit-message regex. Plus a flexible classification system that matches on labels, issue types, and title patterns — with user-defined categories.

### The Problem

The current commit-message regex (`#N`) approach:
- Is too greedy (a commit mentioning `#1` pulls in an ancient issue)
- Misses repos that don't reference issues in commit messages
- Returns 0 issues for repos that link PRs to issues via GitHub's sidebar
- Has no way to discover PRs as work units for PR-centric repos

The current label classification:
- Only matches exact label names (`bug`, `enhancement`)
- Can't match GitHub Issue Types (`type:Bug`)
- Can't match title patterns (`fix:`, `bug:`)
- Fixed to three categories (bug, feature, other) with no customization

## Key Decisions

### 1. Union of All Strategies (Default)

Run all applicable linking strategies and merge results. When multiple strategies find the same issue, that reinforces confidence. The union gives the most complete picture of what shipped.

Strategies:
- **pr-link**: PRs merged between two tags, with their linked issues (via GitHub's timeline/closing references). Most accurate for teams using GitHub's PR-to-issue linking.
- **commit-ref**: Current approach — regex on commit messages for `#N`, `fixes #N`, etc. Works for repos with disciplined commit messages.
- **changelog**: Parse the GitHub Release body for issue/PR references. Good for curated release notes.

No auto-detection or explicit config needed — just run all three and merge.

**Priority merge rule**: When the same issue is discovered by multiple strategies, pr-link wins over commit-ref. A commit saying `refs #42` doesn't mean the issue is fixed; a PR with `closingIssuesReferences` does. For deduplication: PR data (merged_at, linked issues) takes precedence. Commit-ref only contributes issues that pr-link didn't already find. This avoids double-counting and ensures metrics use the most accurate timestamps.

### 2. New `scope` Command

A validation command that shows what the tool discovers before computing metrics:

```
gh-velocity scope v1.5.0 --since v1.4.0 -R owner/repo
```

Output grouped by strategy — shows which issues/PRs each strategy found. Lets users:
- Validate coverage before trusting metrics
- Spot false positives (the `#1` pollution problem)
- Understand which strategy is contributing what
- Debug configuration issues

Supports all output formats (pretty, json, markdown).

### 3. Issues and PRs as First-Class Metric Targets

Issues are the primary unit of work when available. PRs are also first-class — for repos where PRs ARE the work unit (no issues), lead time = PR opened → merged, cycle time = first commit → merged.

The `pr-link` strategy naturally produces both: the PR and its linked issues. When an issue has no linked PR, commit-ref fills the gap.

### 4. User-Defined Classification Categories

Replace the fixed bug/feature/other with arbitrary categories defined in config. Defaults remain bug/feature/other for zero-config experience.

Config syntax:
```yaml
quality:
  categories:
    bug:
      - label:bug
      - label:defect
      - type:Bug
    feature:
      - label:enhancement
      - label:feature
      - type:Feature
    regression:
      - label:regression
      - title:/^regression:/i
    chore:
      - label:chore
      - label:maintenance
```

Matcher types:
- `label:<name>` — issue/PR has a label with this name (case-insensitive)
- `type:<name>` — issue has this GitHub Issue Type
- `title:<regex>` — issue/PR title matches regex (supports `/pattern/flags` syntax)

First match wins (evaluated in config order). Unmatched items fall into "other".

### 5. Inline Flags Override Config

`--bug-match "label:bug,type:Bug"` replaces whatever's in config for that run. Same syntax as config values. To persist, copy the flag value into config. No merging, no surprises.

### 6. Flags and Config Use Identical Syntax

The matcher syntax (`label:bug`, `type:Bug`, `title:/regex/`) is the same whether it appears in a YAML config array or an inline flag. This means the config is self-documenting and flag values can be copy-pasted directly into config.

## Why This Approach

- **Union-all is the most useful for metrics**: More discovered issues = more complete velocity data. Missing issues means undercounting.
- **`scope` command makes it trustworthy**: Users can validate before relying on numbers. Transparency over magic.
- **Strategy pattern is extensible**: Adding a new strategy (e.g., GitHub Projects board, Jira links) is just a new implementation of the interface. No existing code changes.
- **User-defined categories unlock real quality tracking**: Teams can track regressions, tech debt, chores — whatever matters to their process.
- **PR-as-first-class broadens adoption**: Most open source repos don't use issues heavily. Supporting PR-centric workflows makes the tool useful for more repos.

## Resolved Questions

- **Auto-detect vs explicit config vs union?** → Union all strategies by default. No config needed for strategy selection.
- **How should scope output be organized?** → Grouped by strategy (which strategy found what).
- **Should flags merge with or replace config?** → Replace (override). Same syntax, simple mental model.
- **Issues only or PRs too?** → Both are first-class metric targets.
- **Fixed or user-defined categories?** → User-defined with sensible defaults (bug/feature/other).
- **Which match primitives?** → Labels, Issue Types, and title regex from day one.
- **How to fix commit-ref false positives?** → Default to closing keywords only (`fixes`, `closes`, `resolves`). Bare `#N` matching is opt-in via config `commit_ref.patterns: [closes, refs]`. Most precise by default, flexible for teams that want it.
- **Flag naming convention?** → Prefer full flag names (`--repo`, `--format`, `--since`) over short flags in docs and examples. Short aliases are available but not the primary documented form.
- **How to handle duplicate issues across strategies?** → Priority merge: pr-link wins over commit-ref for the same issue. A closing PR reference is a stronger signal than a commit message mention. PR timestamps (merged_at) are used for cycle time when available. Commit-ref only contributes issues not already found by pr-link.
- **Is the PR → linked issues API feasible?** → Yes. GraphQL `closingIssuesReferences` on a PR returns linked issues with full metadata (number, title, state, dates, labels). Tested and working.
- **Will this exhaust API rate limits?** → No. Original per-commit approach (~85 calls) was too heavy. Replaced with search API approach (~22 calls per release run, supports ~225 runs/hour). Key insight: `GET /search/issues?q=is:pr+is:merged+merged:{date}..{date}` finds all merged PRs in a date range in 1 call, avoiding per-commit lookups entirely.
- **Should there be a time window cap?** → Yes. Default 1 month, configurable up to 3 months, hard error beyond that. Prevents runaway API usage on huge release windows.

## Strategies in Detail

### pr-link Strategy

**How it works (verified against GitHub API):**
1. `GET /repos/{owner}/{repo}/compare/{base}...{head}` → list of commit SHAs between tags
2. For each commit: `GET /repos/{owner}/{repo}/commits/{sha}/pulls` → get associated PR number (deduplicate — many commits map to same PR)
3. For each unique PR: GraphQL `closingIssuesReferences(first: 10)` → linked issues with number, title, state, createdAt, closedAt, labels
4. Return: list of (issue, PR, commits) tuples

**GraphQL query for step 3** (tested, works):
```graphql
{
  repository(owner: "...", name: "...") {
    pullRequest(number: N) {
      title
      mergedAt
      closingIssuesReferences(first: 10) {
        nodes { number title state createdAt closedAt labels(first: 10) { nodes { name } } }
      }
    }
  }
}
```

**Cycle time**: First commit on PR branch → PR merged
**Lead time**: Issue created → issue closed (or PR merged if no issue)

**API cost**: Low — uses search API instead of per-commit lookups.

**Efficient approach (verified):**
- Skip the per-commit → PR mapping entirely (that's 1 REST call per commit — too expensive)
- Instead: `GET /search/issues?q=repo:{owner}/{repo}+is:pr+is:merged+merged:{start}..{end}` returns all merged PRs in the date range in 1 paginated call
- Then batch GraphQL for `closingIssuesReferences` across all PRs in 1-2 calls
- Total for a typical release: **~22 API calls** (vs ~85 with per-commit approach)
- At 5,000 calls/hour limit: supports ~225 runs/hour

**GraphQL batching**: Fetch multiple PRs' linked issues in a single query using aliases:
```graphql
{ repository(owner: "...", name: "...") {
    pr101: pullRequest(number: 101) { closingIssuesReferences(first: 10) { ... } }
    pr102: pullRequest(number: 102) { closingIssuesReferences(first: 10) { ... } }
} }
```

### commit-ref Strategy

**How it works (refined from current implementation):**
1. Get commits between two tags
2. By default, only match **closing keywords**: `fixes #N`, `closes #N`, `resolves #N` (and their variants). This eliminates the greedy `#N` matching that caused false positives.
3. Additional patterns are configurable — teams that use bare `#N` deliberately can opt in.
4. Fetch referenced issues
5. Return: list of (issue, commits) tuples

**Default patterns**: `closes`, `fixes`, `resolves` (and variants like `closed`, `fixed`, `resolved`)
**Opt-in patterns via config**:
```yaml
commit_ref:
  patterns:
    - closes    # fixes #N, closes #N, resolves #N (default)
    - refs      # bare #N references (opt-in, greedy)
```

This means out of the box, a commit saying "update step #1" does NOT link to issue #1. Only "fixes #1" or "closes #1" would match. Teams that want bare `#N` matching add `refs` to their config.

### changelog Strategy

**How it works:**
1. Fetch the GitHub Release body for the target tag
2. Parse for issue/PR references (`#N`, `owner/repo#N`, full URLs)
3. Fetch referenced issues/PRs
4. Return: list of (issue/PR) tuples

**Useful for**: Repos with curated release notes that explicitly list what shipped.

### 7. Time Window Guardrails

To prevent abusive API consumption:
- **Default max window**: 1 month between `--since` tag and target tag
- **Configurable up to**: 3 months (`max_window_days: 90` in config)
- **Beyond 3 months**: Error with a clear message suggesting narrowing the range
- This also prevents the "first release with no --since scans entire history" problem
- The `scope` command respects the same limits

## Out of Scope (for now)

- GitHub Projects v2 board integration (status field transitions for lead time)
- Jira/Linear integration
- PR review metrics (review time, rounds of review)
- Custom linking patterns beyond the three strategies
- Weighted confidence scoring across strategies
