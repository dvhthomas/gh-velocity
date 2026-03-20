---
title: "feat: Single-item detail commands (issue and pr)"
type: feat
status: completed
date: 2026-03-19
origin: docs/brainstorms/2026-03-19-single-item-detail-commands-requirements.md
---

# feat: Single-item detail commands (`issue` and `pr`)

## Overview

Add top-level `gh velocity issue <N>` and `gh velocity pr <N>` commands that show composite detail views of a single item — facts, metrics, related items, and provenance in one comment. These replace the need to run multiple `flow` commands and produce genuinely useful posted comments.

## Problem Statement / Motivation

Current single-item commands (`flow lead-time 42`, `flow cycle-time --pr 125`) each show one metric in isolation. Posted comments are barely useful — no timestamps, no linked PRs, no category. The bulk `report` shows everything but for a time window. There's no "tell me everything about this one item" command. (See origin: `docs/brainstorms/2026-03-19-single-item-detail-commands-requirements.md`)

## Proposed Solution

Two new top-level commands alongside `report`:

```
gh velocity issue 42          # Composite issue detail
gh velocity pr 125            # Composite PR detail
gh velocity issue 42 --post   # Post rich comment to issue
gh velocity pr 125 --post     # Post rich comment to PR
```

Each implements the `Pipeline` interface (`GatherData` → `ProcessData` → `Render`) in its own package under `internal/pipeline/`.

## Technical Approach

### Phase 1: Foundation (issue command — core metrics)

Create `issue <N>` with lead time, cycle time, category, and linked PRs.

**New files:**
- `cmd/issue.go` — Cobra command, wiring, `renderPipeline` call
- `internal/pipeline/issue/issue.go` — Pipeline struct, GatherData, ProcessData
- `internal/pipeline/issue/render.go` — Format-specific rendering (pretty, markdown, json, html)
- `internal/pipeline/issue/issue_test.go` — Table-driven tests

**Changes to existing files:**
- `cmd/root.go` — Add `root.AddCommand(NewIssueCmd())`
- `internal/github/cyclestart.go` — Change `GetClosingPR` → `GetClosingPRs` returning `[]*model.PR` (iterate all `CLOSED_EVENT` nodes instead of returning first match)

**Data flow:**
1. `GatherData`: Fetch issue via `GetIssue()`, fetch closing PRs via `GetClosingPRs()`, compute cycle time via existing strategy (labels → project board fallback)
2. `ProcessData`: Compute lead time, classify category via `classify.Classify()`, compute each closing PR's cycle time (created → merged)
3. `Render`: Output facts block (created, closed, category), metrics table (lead time, cycle time), related PRs table

**Output shape (markdown):**
```markdown
## Issue #119: feat(preflight): auto-detect and exclude noise labels

**Created:** 2026-03-18 18:55 UTC · **Closed:** 2026-03-18 18:56 UTC · **Category:** feature

| Metric | Value |
|--------|-------|
| Lead Time | 1m (created → closed) |
| Cycle Time | — (no in-progress signal) |

### Linked PRs
| PR | Title | Cycle Time |
|----|-------|------------|
| #120 | docs: noise exclusion guide | 14m (created → merged) |
```

**Output shape (JSON):**
```json
{
  "issue": {
    "number": 119,
    "title": "feat(preflight): auto-detect and exclude noise labels",
    "url": "https://github.com/dvhthomas/gh-velocity/issues/119",
    "created_at": "2026-03-18T18:55:00Z",
    "closed_at": "2026-03-18T18:56:07Z",
    "category": "feature"
  },
  "metrics": {
    "lead_time": {
      "seconds": 67,
      "display": "1m",
      "signal": "created -> closed"
    },
    "cycle_time": {
      "status": "not_applicable",
      "reason": "no in-progress signal configured"
    }
  },
  "linked_prs": [
    {
      "number": 120,
      "title": "docs: noise exclusion guide",
      "url": "https://github.com/dvhthomas/gh-velocity/pull/120",
      "cycle_time": {
        "seconds": 840,
        "display": "14m",
        "signal": "created -> merged"
      }
    }
  ],
  "provenance": { "command": "issue", "repo": "dvhthomas/gh-velocity", "config": {} },
  "warnings": []
}
```

**Three-state metric rendering (R6):**
- Completed: `14m (created → merged)`
- In progress: `in progress since 2026-03-18`
- Not applicable: `— (no in-progress signal)` with JSON `{"status": "not_applicable", "reason": "..."}`

Reason strings per metric:
| Metric | N/A Reason |
|--------|-----------|
| Lead time (open issue) | `issue still open` |
| Cycle time (no lifecycle config) | `no in-progress signal configured` |
| Cycle time (no label/board match) | `no in-progress event found` |
| Linked PR cycle time (not merged) | `PR not merged` |

### Phase 2: PR command with review metrics

Create `pr <N>` with cycle time, review metrics, and closed issues.

**New files:**
- `cmd/pr.go` — Cobra command
- `internal/pipeline/pr/pr.go` — Pipeline struct
- `internal/pipeline/pr/render.go` — Format-specific rendering
- `internal/pipeline/pr/pr_test.go` — Table-driven tests
- `internal/github/reviews.go` — New GraphQL query for PR review timeline

**Changes to existing files:**
- `cmd/root.go` — Add `root.AddCommand(NewPRCmd())`
- `internal/github/prs.go` — Populate `Author` field in `GetPR()` response
- `internal/model/types.go` — Add `ReviewSummary` struct

**New GraphQL query** (`internal/github/reviews.go`):
```graphql
query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviews(first: 100) {
        nodes {
          author { login }
          state          # APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
          submittedAt
        }
      }
      timelineItems(first: 100, itemTypes: [REVIEW_REQUESTED_EVENT]) {
        nodes {
          ... on ReviewRequestedEvent {
            createdAt
            requestedReviewer { ... on User { login } }
          }
        }
      }
    }
  }
}
```

**Review metric definitions:**

| Metric | Definition |
|--------|-----------|
| Time to first review | PR `createdAt` → first `review.submittedAt` (any state except COMMENTED-only) |
| Review rounds | Count of non-comment review submissions (APPROVED or CHANGES_REQUESTED). A round = one substantive review. |
| Wait time | Sum of gaps where PR was waiting for reviewer feedback: from PR creation (or last author push) to each review submission. Approximated as `time_to_first_review + time_between_subsequent_reviews`. |

**Author type detection** (read-only signals, no mutations):
- `bot`: Author login ends with `[bot]` OR author login appears in `exclude_users` config
- `agent-assisted`: Any commit on the PR has a `Co-Authored-By` trailer matching known AI patterns (`noreply@anthropic.com`, `noreply@github.com` from Copilot)
- `human`: Neither of the above

Present in JSON output as `"author_type": "human|bot|agent-assisted"`. In pretty/markdown output, only surface when `bot` or `agent-assisted` (append to author line).

**Output shape (markdown):**
```markdown
## PR #125: feat: HTML format, insight flags, and cleanup

**Author:** dvhthomas · **Opened:** 2026-03-19 01:49 UTC · **Merged:** 2026-03-19 02:16 UTC

| Metric | Value |
|--------|-------|
| Cycle Time | 27m (created → merged) |
| Time to First Review | 12m |
| Review Rounds | 1 |

### Closed Issues
| Issue | Title |
|-------|-------|
| #119 | feat(preflight): auto-detect and exclude noise labels |
```

### Phase 3: Workflow update and docs

- Update `.github/workflows/velocity-item.yaml` to use `issue` and `pr` commands
- Update `docs/single-item-workflow.md` to reference the new commands
- Add smoke tests for both commands

## Partial Failure Strategy

| Data source | Failure behavior |
|-------------|-----------------|
| Issue/PR fetch (`GetIssue`/`GetPR`) | **Fatal** — exit code 4 (not found) or 1 (API error) |
| Closing PRs lookup | **Degrade** — omit related section, add warning |
| Cycle time signals (labels/board) | **Degrade** — show `— (reason)`, add warning |
| Review data fetch | **Degrade** — show `— (review data unavailable)`, add warning |
| Category classification | **Degrade** — show `other` (existing fallback) |

## Acceptance Criteria

- [ ] `gh velocity issue <N>` shows facts (created, closed, category), metrics (lead time, cycle time), and linked PRs with cycle times (R1, R3)
- [ ] `gh velocity pr <N>` shows facts (opened, merged, author), metrics (cycle time, time-to-first-review, review rounds), and closed issues (R2, R4)
- [ ] Facts and metrics are clearly separated in all output formats (R5)
- [ ] Unavailable metrics show `—` with a reason in pretty/markdown, structured status in JSON (R6)
- [ ] `--post` uses single idempotent markers: `gh-velocity:issue:N`, `gh-velocity:pr:N` (R7)
- [ ] All formats work: `--results pretty`, `markdown`, `json`, `html` (R8)
- [ ] Existing `flow lead-time` and `flow cycle-time` commands continue to work unchanged
- [ ] `GetClosingPRs` returns all closing PRs (not just first)
- [ ] `GetPR` populates the `Author` field
- [ ] PR author type (`human`, `bot`, `agent-assisted`) is present in JSON output
- [ ] Table-driven tests cover: open items, closed items, no review data, bot-authored, multiple closing PRs, missing lifecycle config
- [ ] `.github/workflows/velocity-item.yaml` updated to use new commands (R9)
- [ ] `docs/single-item-workflow.md` updated (R9)

## Dependencies & Risks

- **Review timeline GraphQL fields**: Need to verify `reviews` and `timelineItems(REVIEW_REQUESTED_EVENT)` are available and return expected data. If review round detection is unreliable, ship without it and add later.
- **`GetClosingPRs` changes**: Modifying `GetClosingPR` affects existing callers in the cycle-time pipeline. Must ensure backwards compatibility or update all callers.
- **Scope creep**: The PR review metrics (time-to-first-review, rounds, wait time) are net-new. If the GraphQL data model doesn't support clean computation, descope review metrics to Phase 2b and ship the PR command with just cycle time + closed issues first.

## Sources & References

### Origin

- **Origin document:** [docs/brainstorms/2026-03-19-single-item-detail-commands-requirements.md](docs/brainstorms/2026-03-19-single-item-detail-commands-requirements.md) — Key decisions: top-level commands, facts vs metrics separation, PR depth with review metrics, agent detection via read-only signals.

### Internal References

- Pipeline interface: `internal/pipeline/pipeline.go`
- Render pipeline: `cmd/render.go`
- Post system: `cmd/post.go`, `internal/posting/poster.go`, `internal/posting/marker.go`
- Report composite example: `cmd/report.go`
- Lead time single: `cmd/leadtime.go:66`, `internal/pipeline/leadtime/leadtime.go`
- Cycle time single: `cmd/cycletime.go:108`, `internal/pipeline/cycletime/cycletime.go`
- GetClosingPR: `internal/github/cyclestart.go:48`
- FetchPRLinkedIssues: `internal/github/pullrequests.go:78`
- Three-state metric: `docs/solutions/three-state-metric-status-pattern.md`
- Output shape: `docs/solutions/architecture-patterns/command-output-shape.md`
- Cycle time signals: `docs/solutions/cycle-time-signal-hierarchy.md`
- Existing workflow: `.github/workflows/velocity-item.yaml`
- Existing guide: `docs/single-item-workflow.md`
