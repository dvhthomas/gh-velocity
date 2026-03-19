---
date: 2026-03-19
topic: single-item-detail-commands
---

# Single-Item Detail Commands (`issue` and `pr`)

## Problem Frame

The current single-item commands (`flow lead-time <N>`, `flow cycle-time --pr <N>`) each show one metric in isolation. When posted as GitHub comments, they're barely useful — e.g., lead time shows "1m (created → closed)" with no close timestamp, no cycle time, no linked PRs, no category. Meanwhile the bulk `report` command shows everything but for a time window, not a single item. There's no middle ground: "tell me everything about this one item."

Users who set up the per-item GitHub Actions workflow (issue close → comment, PR merge → comment) need a single rich comment per item, not multiple thin ones.

## Requirements

- R1. New top-level `issue <N>` command that shows a composite detail view of a single issue, posted as one comment with one idempotent marker.
- R2. New top-level `pr <N>` command that shows a composite detail view of a single PR, posted as one comment with one idempotent marker.
- R3. `issue <N>` output includes:
  - Facts: created timestamp, closed timestamp, category (from config matchers)
  - Metrics: lead time (created → closed), cycle time (when available from labels or project board)
  - Related: linked PRs that close this issue, with each PR's cycle time (created → merged)
- R4. `pr <N>` output includes:
  - Facts: opened timestamp, merged timestamp, author
  - Metrics: cycle time (created → merged), time to first review, review rounds, wait time (time PR spent not being actively reviewed)
  - Related: issues closed by this PR (with links)
  - Note: per-PR review data (author, wait time, review turnaround) should be structured in JSON output to support future aggregate analysis — e.g., slicing throughput by author to surface review bottlenecks and highest wait-time authors. Detection of agent-authored PRs uses read-only signals: co-authored-by commit trailers, `[bot]` username suffix, `exclude_users` config list.
- R5. Facts (timestamps, category) and metrics (lead time, cycle time) are clearly separated in the output — "Created" is a fact, "Lead Time" is a derived metric.
- R6. When a metric is unavailable (e.g., no in-progress signal for cycle time), show a dash with a brief reason rather than omitting it silently.
- R7. Both commands support `--post` with single idempotent markers (`<!-- gh-velocity:issue:119 -->`, `<!-- gh-velocity:pr:125 -->`).
- R8. Both commands support all existing output formats (`--results pretty`, `markdown`, `json`, `html`).
- R9. Update the `velocity-item.yaml` GitHub Actions workflow to use the new commands instead of `flow lead-time` and `flow cycle-time --pr`.

## Success Criteria

- A single `gh velocity issue 119 --post` replaces the need to run multiple flow commands.
- The posted comment is genuinely useful — a reader can understand the full lifecycle of that issue from one comment.
- The `pr` command adds value beyond what GitHub's UI already shows (review timing, cycle time).
- The Actions workflow produces one rich comment per item, not multiple thin ones.

## Scope Boundaries

- No changes to the existing `flow` subcommands — they remain for bulk analysis and metric-specific drill-downs.
- No label-sync automation (project board → labels) — that's a separate effort.
- No new data sources — use existing API calls and config infrastructure.
- Do not reproduce information GitHub already shows in its UI (linked issues sidebar, PR reviewers list). Add only computed metrics and timestamps that GitHub doesn't surface.

## Key Decisions

- **Top-level commands**: `issue` and `pr` sit alongside `report`, not under `flow`. They're composite views, not single-metric drill-downs.
- **Facts vs metrics separation**: Output clearly distinguishes timestamps/classification (facts) from computed values (metrics). "Created" is not a "Metric."
- **Unavailable metrics show dash + reason**: Rather than silently omitting, show `— (no in-progress signal)` so users know what's missing and why.
- **PR command has depth**: Not just cycle time — includes review turnaround metrics (time to first review, review rounds) that GitHub's UI doesn't compute.
- **Linked PR resolution**: `issue` command resolves closing PRs via `closingIssuesReferences` (reverse lookup) and includes their cycle times. Existing `BuildClosingPRMap` pattern can be adapted.

## Dependencies / Assumptions

- Linked PR lookup requires the `closingIssuesReferences` GraphQL field, which is already used in `internal/github/pullrequests.go`. Need to verify that the reverse direction (issue → closing PRs) is available via `closedByPullRequestsReferences`.
- Review metrics (time to first review, review rounds) require PR timeline data — verify this is accessible via existing API patterns.
- Cycle time availability depends on lifecycle config (labels or project board). The command should gracefully degrade when neither is configured.

## Outstanding Questions

### Deferred to Planning

- [Affects R3][Needs research] Does `closedByPullRequestsReferences` GraphQL field exist on Issue type? Earlier test returned empty — need to verify if this is a field availability issue or just no linked PRs for that issue.
- [Affects R4][Needs research] What PR timeline events are available for computing time-to-first-review, review rounds, and wait time? Check if `PullRequestReviewEvent` or `ReviewRequestedEvent` timeline items are accessible. Also verify that "review requested" → "first review submitted" gap can be computed.
- [Affects R4][Needs research] What read-only signals reliably indicate agent-authored PRs? Verify: co-authored-by trailer parsing from commit messages, `[bot]` username suffix, intersection with `exclude_users` config. Check if GitHub exposes any native "app-authored" or "bot" flag on PR author.
- [Affects R3, R4][Technical] Should the commands reuse existing pipeline infrastructure (like `leadtime.Pipeline`, `cycletime.Pipeline`) internally, or build a new composite pipeline?
- [Affects R8][Technical] JSON schema for the composite output — flat object with nested sections, or separate top-level keys for facts/metrics/related?

## Related

- Label-sync automation (project board → labels) is a separate effort that would improve cycle-time accuracy for the `issue` command.
- The existing `single-item-workflow.md` guide and `velocity-item.yaml` workflow will need updating once these commands ship.

## Next Steps

→ `/ce:plan` for structured implementation planning
