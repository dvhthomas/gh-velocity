---
date: 2026-03-19
topic: label-sync-and-board-removal
---

# Label Sync Action + Project Board Lifecycle Removal

## Problem Frame

GitHub Projects v2 `updatedAt` timestamps on status fields are unreliable for measuring when work started — they reflect the last field update, not the original status transition. This makes project-board-based cycle-time fundamentally broken. The workaround (label-based lifecycle signals) works but requires users who manage work via the project board to manually apply labels, which is error-prone and forgotten.

Two coordinated changes fix this:
1. A standalone GitHub Action that keeps issue labels and project board status in sync (bidirectional, scheduled)
2. Remove project board as a lifecycle signal from gh-velocity entirely — labels are the sole source of truth for cycle-time and WIP

## Part A: Standalone Label-Sync Action

### Requirements

- R1. New standalone GitHub Action repository (separate from gh-velocity) that bidirectionally syncs issue labels with GitHub Projects v2 status field values.
- R2. Runs on a cron schedule (configurable, default every 15 minutes). Not event-driven — `projects_v2_item` is not available as an Actions trigger.
- R3. For each open issue in the configured project, compares the Status field value to issue labels. Reconciles in both directions:
  - Board status changed → apply matching label, remove competing status labels
  - Label added/removed → update board status to match
- R4. Conflict resolution: most-recent-write-wins based on timestamps (label `createdAt` vs board item `updatedAt`). If timestamps are identical, board wins (as the richer data source).
- R5. Label naming convention is configurable. Default: `status:<StatusName>` (e.g., `status:In Progress`). User can override the prefix.
- R6. Scoped to a single project (by URL or project number) and a single repo. Does not sync across repos.
- R7. Only processes open issues (closed issues are skipped). Draft items without an associated issue are skipped.
- R8. Creates missing labels automatically (with a neutral color) on first sync.
- R9. Requires a classic PAT or GitHub App token with `project` + `repo` scopes. Documents this clearly — `GITHUB_TOKEN` cannot access project board data.
- R10. Works for both user-owned and organization-owned projects.
- R11. Dry-run mode by default (logs what would change, no mutations). Live mode requires explicit opt-in via input parameter.

### Success Criteria

- A user with a GitHub Projects board can install this Action and have labels mirror board status within 15 minutes of any change.
- The Action is useful on its own — it is not coupled to gh-velocity in any way.

### Implementation Constraints

- Written in Go. Single binary, cross-compiled via goreleaser for the Action.
- Prefer `gh` CLI for operations where it works (label management, simple queries). Resort to custom Go + GraphQL only where `gh` falls short (batch project item reads, field value updates).
- Robust API handling: rate limit detection with backoff, retry on transient failures (5xx, network errors), respect `Retry-After` headers.
- Detailed logging: every sync decision logged (what changed, what was skipped, why). Structured output suitable for Actions log viewing.
- Dry-run by default — live mode requires explicit opt-in input parameter.

### Scope Boundaries

- Not part of the gh-velocity codebase — standalone repo and Action.
- Not a prerequisite for gh-velocity. gh-velocity reads labels regardless of how they were applied (manually, via this Action, via any other automation).
- Does not handle PR status sync (issues only).
- Does not handle multiple projects per repo.
- Does not attempt real-time sync (no webhook relay infrastructure).

## Part B: gh-velocity Project Board Lifecycle Removal

### Requirements

- R12. Remove `lifecycle.*.project_status` config field entirely. Breaking change — no deprecation shim.
- R13. Remove all cycle-time computation code that uses project board status (`computeFromProject`, `GetProjectStatus`, `BatchGetProjectStatuses`, `fetchProjectStatus`, `fetchProjectStatusBatch`, `matchProjectStatus`).
- R14. Cycle-time issue strategy uses only label-based signals (`lifecycle.*.match` via `GetLabelCycleStart`).
- R15. WIP command (`status wip`) queries for open issues with matching lifecycle labels instead of querying the project board. Uses `lifecycle.in-progress.match` and `lifecycle.in-review.match` config fields.
- R16. Keep `project.url` and `project.status_field` config for velocity iteration/effort strategies that read current board field values (not timestamps).
- R17. Config validation: remove project_status-related validation rules. Add validation that cycle-time issue strategy requires `lifecycle.in-progress.match`.
- R18. Update `config preflight --write` to stop generating `project_status` in lifecycle stages. Generate `match` patterns instead (label-based).
- R19. Update `defaultConfigTemplate` to remove project_status examples and show label-based lifecycle instead.
- R20. Update all documentation: guide.md, site/content/, example configs. Remove project board cycle-time references. Update "labels vs board" guide to reflect that labels are now the only option (not a recommendation).
- R21. Update `.gh-velocity.yml` for the dvhthomas/gh-velocity repo to use label-based lifecycle instead of project_status.

### Success Criteria

- `lifecycle.*.project_status` is not recognized in config (or silently ignored with a warning).
- Cycle-time is computed exclusively from label signals.
- WIP command works with label queries instead of board queries.
- All tests pass with no project-board-as-lifecycle references.
- Documentation is consistent — no mentions of project_status for lifecycle.

### Scope Boundaries

- Velocity iteration and effort strategies that use the project board are NOT removed.
- `project.url` and `project.status_field` remain in config for these strategies.
- `ResolveProject` and `ListProjectItems` GraphQL code stays (used by velocity).

## Key Decisions

- **Breaking change, no shim**: No users to protect. Full cleanup.
- **Labels are sole lifecycle signal**: `lifecycle.*.match` (labels) is the only way to define workflow stages for cycle-time and WIP.
- **Bidirectional scheduled sync**: The standalone Action polls every 15 minutes. Not event-driven due to GitHub API limitations (`projects_v2_item` is not an Actions trigger; user-owned projects have no webhooks).
- **Separate repo, not a dependency**: The label-sync Action is independent — useful beyond gh-velocity, simpler maintenance, clear separation of concerns. gh-velocity is read-only and doesn't care how labels are applied. The Action is a recommended convenience for project board users, not a prerequisite.
- **Project board kept for current-state reads**: Velocity iteration/effort strategies continue to use the board. Only lifecycle/cycle-time signals are removed.

## Research Findings

No existing maintained Action does bidirectional project-status ↔ label sync for Projects v2. Key constraints discovered:

- `projects_v2_item` is NOT a GitHub Actions trigger — it only fires as an org-level webhook
- User-owned projects have no webhooks at all — polling is the only option
- `GITHUB_TOKEN` cannot access project board data — a classic PAT with `project` + `repo` scopes is required
- Fine-grained PATs do not support the Projects v2 GraphQL API
- Existing building blocks (`EndBug/project-fields`, `github/update-project-action`) handle individual field reads/writes but have no sync logic
- `estruyf/github-project-labeling` (0 stars, Azure Functions, not a GitHub Action) is the only prior art for board→labels direction

## Dependencies / Assumptions

- The label-sync Action needs a classic PAT with `project` + `repo` scopes. `GITHUB_TOKEN` cannot access project data.
- Both directions use cron-based polling since event-driven board→labels is not possible in Actions.
- Users who don't use project boards at all are unaffected — they already use labels.

## Outstanding Questions

### Deferred to Planning

- [Affects R2][Technical] What's the optimal polling interval? 5 minutes burns more API quota but feels more responsive. 15 minutes is conservative. Should this be configurable with a sensible default?
- [Affects R3][Needs research] How many API calls does a full board scan require? For a board with 100 open issues, how close to the rate limit does a single reconciliation run get?
- [Affects R4][Technical] Is label `createdAt` available via REST, or only via GraphQL timeline events? The conflict resolution logic needs both timestamps efficiently.
- [Affects R15][Technical] How does WIP transition from board query to label query? Does `gh issue list -l "status:In Progress"` give the same result as the board query? What about issues in the project that aren't labeled yet (first sync hasn't run)?
- [Affects R13][Technical] Which GraphQL functions in `internal/github/cyclestart.go` are shared with non-lifecycle code? Need to confirm nothing else calls `GetProjectStatus` before deleting.

## Next Steps

→ `/ce:plan` for structured implementation planning (two plans: one for the Action, one for the gh-velocity cleanup)
