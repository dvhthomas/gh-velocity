---
title: "feat: Command Hierarchy Redesign — Bulk, Stats, WIP, Quality"
type: feat
status: completed
date: 2026-03-10
---

# Command Hierarchy Redesign

## Overview

Redesign the gh-velocity command tree to support single-item and bulk modes on the same commands, add `stats` (dashboard), `wip` (work in progress), and rename `release` to `quality release`. All times are explicitly UTC.

Brainstorm: `docs/brainstorms/2026-03-10-command-hierarchy-brainstorm.md`

## Problem Statement

The current CLI only supports single-item queries (`lead-time 42`) or release-scoped aggregates (`release v1.0`). Teams need:
- Time-filtered bulk queries: "lead time for all issues closed in the last 30 days"
- A dashboard view: "how are we doing overall?"
- WIP visibility: "what's in progress right now?"
- Quality metrics properly scoped: release-bounded, not conflated with velocity

## Proposed Solution

Same command, different args. `--since DATE` is the universal bulk trigger. A new `stats` command composes all metrics into a trailing-window dashboard. `wip` shows in-progress work from Projects v2 or labels.

### Command Tree

```
gh velocity
  lead-time <issue>                        # single issue
  lead-time --since DATE [--until DATE]    # bulk: closed issues in window

  cycle-time <issue>                       # single issue (configured strategy)
  cycle-time --pr <number>                 # single PR
  cycle-time --since DATE [--until DATE]   # bulk: closed issues/PRs in window

  quality release <tag> [--since <tag>]    # release-scoped quality metrics

  wip [-R owner/repo]                      # current work in progress

  stats [--since DATE] [--until DATE]      # 30-day dashboard (default)

  scope <tag> [--since <tag>]              # diagnostic (unchanged)
  config show | validate | create | discover
  version
```

## Implementation Phases

### Phase 1: Foundation — Date Parsing & UTC Enforcement

Shared infrastructure that all subsequent phases depend on.

#### 1a. Date parsing utility

- [x] Create `internal/dateutil/parse.go`
- [x] Create `internal/dateutil/parse_test.go`

```go
// internal/dateutil/parse.go
package dateutil

// Parse accepts YYYY-MM-DD, RFC3339, or relative (Nd).
// All returned times are UTC.
// YYYY-MM-DD → start of day UTC.
// Nd → now.UTC() minus N days.
func Parse(s string, now time.Time) (time.Time, error)

// ValidateWindow checks since < until and since is not in the future.
func ValidateWindow(since, until time.Time) error
```

Supported formats:
- `2026-01-15` → `2026-01-15T00:00:00Z`
- `2026-01-15T14:30:00Z` → as-is, converted to UTC
- `30d` → `now.UTC().AddDate(0, 0, -30)`
- `0d` → start of today UTC

Validation:
- Reject future `--since` dates
- Reject `--since` >= `--until` (inverted or zero-width window)
- Reject malformed input with clear error: `invalid date "foo": expected YYYY-MM-DD, RFC3339, or relative (e.g., 30d)`

Table-driven tests for: valid dates, relative dates, `0d`, future dates, inverted windows, leap year non-dates (`2026-02-29`), invalid month (`2026-13-01`).

#### 1b. UTC enforcement across codebase

- [x] Fix `internal/github/pullrequests.go:39` — add `.UTC()` before formatting search dates
- [x] Fix `internal/git/git.go` — add `.UTC()` after parsing commit timestamps
- [x] Ensure all JSON output timestamps use `.UTC().Format(time.RFC3339)`
- [x] Pretty/markdown output: append ` UTC` to displayed timestamps

Reference: `internal/github/pullrequests.go:39`, `internal/git/git.go:132`

---

### Phase 2: Bulk Search API

New GitHub client method for querying closed issues by date range.

- [x] Create `internal/github/search.go`
- [x] Create `internal/github/search_test.go` (covered by E2E tests, matching SearchMergedPRs pattern)

```go
// internal/github/search.go

// SearchClosedIssues finds all issues closed in the given date range.
// Uses: GET /search/issues?q=repo:{owner}/{repo}+is:issue+is:closed+closed:{start}..{end}
// Returns at most 1000 results (GitHub search API limit).
// Warns on stderr if results are capped.
// Returns model.Issue with fields populated from search results (number, title, state, createdAt, closedAt).
func (c *Client) SearchClosedIssues(ctx context.Context, since, until time.Time) ([]model.Issue, error)
```

Pattern: clone `SearchMergedPRs` in `internal/github/pullrequests.go:38-85`. Same pagination (100/page, max 10 pages), same error handling. Key difference: query uses `is:issue+is:closed+closed:{start}..{end}` instead of `is:pr+is:merged+merged:{start}..{end}`.

Why Search API (not REST Issues endpoint): The REST `since` parameter filters by `updated_at`, not `closed_at`. An issue updated today but closed 6 months ago would appear in a 7-day window — incorrect results.

---

### Phase 3: Bulk Mode for lead-time and cycle-time

Add `--since`/`--until` flags to existing commands with auto-switch.

#### 3a. lead-time bulk

- [x] Modify `cmd/leadtime.go` — add `--since`/`--until` flags, change `Args` to `MaximumNArgs(1)`
- [x] Add conflict detection: error if both positional arg AND `--since` are given
- [x] Bulk path: `SearchClosedIssues` → per-issue `metrics.LeadTime` → `metrics.ComputeStats`
- [x] Add bulk formatters in `internal/format/` for all three output modes

```go
// cmd/leadtime.go — RunE logic
if len(args) > 0 && sinceFlag != "" {
    return error("provide either an issue number or --since, not both")
}
if sinceFlag != "" {
    return runLeadTimeBulk(cmd, sinceFlag, untilFlag)
}
if len(args) == 0 {
    return error("provide an issue number or use --since for bulk mode")
}
return runLeadTimeSingle(cmd, args[0])
```

Bulk output shape (JSON):
```json
{
  "repository": "owner/repo",
  "window": { "since": "2026-01-01T00:00:00Z", "until": "2026-02-01T00:00:00Z" },
  "items": [
    { "number": 42, "title": "...", "lead_time": { "start": {...}, "end": {...}, "duration_seconds": 172800 } }
  ],
  "stats": { "count": 14, "mean_seconds": 259200, "median_seconds": 172800, "p90_seconds": 604800 }
}
```

Pretty output: table with per-item rows + stats summary row. Sort by close date descending.

Aggregate display rules (existing `Stats` behavior):
- Always show count
- Show median when count >= 2
- Show P90 when count >= 5

#### 3b. cycle-time bulk

- [x] Modify `cmd/cycletime.go` — same pattern as lead-time
- [x] Conflict detection: error if positional arg + `--since`, or `--pr` + `--since`
- [x] Bulk path: `SearchClosedIssues` → per-issue `cycletime.Strategy.Compute` → `metrics.ComputeStats`
- [x] Per-strategy bulk behavior (all three are peer strategies with the same pattern):
  - `issue`: start = `createdAt` (already on issue), end = `closedAt` — no extra API calls
  - `pr`: start = closing PR `createdAt`, end = PR `mergedAt` — 1 REST call per issue (`GetClosingPR`)
  - `project-board`: start = status departure from backlog, end = `closedAt` — 1 GraphQL call per issue (board status history)
- [x] Items where the start signal is unavailable (no closing PR, not on board) → cycle time is N/A, included in output with `null` duration, excluded from stats

---

### Phase 4: quality release (Rename)

- [x] Create `cmd/quality.go` — parent command with `release` as subcommand
- [x] Move release logic: `NewReleaseCmd()` stays in `cmd/release.go`, wired as child of `quality`
- [x] Update `cmd/root.go` — replace `root.AddCommand(NewReleaseCmd())` with `root.AddCommand(NewQualityCmd())`
- [x] Keep `release` as hidden deprecated alias on root: prints warning, delegates to `quality release`
- [x] Update smoke tests and E2E tests

```go
// cmd/quality.go
func NewQualityCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "quality",
        Short: "Quality metrics for releases",
    }
    cmd.AddCommand(NewReleaseCmd())
    return cmd
}
```

```go
// cmd/root.go — deprecated alias
deprecatedRelease := NewReleaseCmd()
deprecatedRelease.Hidden = true
deprecatedRelease.Deprecated = "use 'quality release' instead"
root.AddCommand(deprecatedRelease)
```

Issue #9 tracks this rename.

---

### Phase 5: wip Command

New command showing in-progress work.

#### 5a. Projects v2 item listing

- [ ] Create `internal/github/projectitems.go`
- [ ] Create `internal/github/projectitems_test.go`

```go
// ProjectItem represents an item on a Projects v2 board.
type ProjectItem struct {
    ContentType string     // "Issue", "PullRequest", or "DraftIssue"
    Number      int        // 0 for drafts
    Title       string
    Repo        string     // "owner/repo" from content.repository.nameWithOwner
    Status      string     // current board status
    StatusAt    *time.Time // when status was last set (updatedAt on field value)
    CreatedAt   time.Time
    Labels      []string
}

// ListProjectItems returns all items on a Projects v2 board.
// Uses cursor-based pagination with node(id:) query.
func (c *Client) ListProjectItems(ctx context.Context, projectID, statusFieldID string) ([]ProjectItem, error)
```

GraphQL query (variables only, never interpolation):
```graphql
query($projectId: ID!, $cursor: String) {
  node(id: $projectId) {
    ... on ProjectV2 {
      items(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          content {
            __typename
            ... on Issue {
              number title state createdAt
              repository { nameWithOwner }
              labels(first: 20) { nodes { name } }
            }
            ... on PullRequest {
              number title state createdAt
              repository { nameWithOwner }
            }
            ... on DraftIssue { title createdAt }
          }
          fieldValues(first: 20) {
            nodes {
              __typename
              ... on ProjectV2ItemFieldSingleSelectValue {
                name updatedAt
                field { ... on ProjectV2SingleSelectField { id } }
              }
            }
          }
        }
      }
    }
  }
}
```

#### 5b. WIP command

- [x] Create `cmd/wip.go`
- [x] Wire in `cmd/root.go`

Logic:
1. If `config.Project.ID` is set → `ListProjectItems`, filter where status is NOT `config.Statuses.Backlog` and NOT `config.Statuses.Done`
2. If `-R owner/repo` is given → additionally filter by `item.Repo`
3. Else if `config.Statuses.ActiveLabels` is non-empty → search for open issues with those labels, exclude any with backlog_labels
4. Else → error: `wip requires either project.id or statuses.active_labels in .gh-velocity.yml`

Output per item:
- Number, title, status, age (time since `StatusAt` or `CreatedAt`), repo (when showing cross-repo board)

Age = `now.Sub(statusAt)` if available, else `now.Sub(createdAt)` (durations are timezone-agnostic)

Note: `statusAt` reflects the LAST status change (not first departure from backlog). This is a known Projects v2 API limitation documented in `docs/solutions/cycle-time-signal-hierarchy.md`.

---

### Phase 6: stats Command

Dashboard composing all metrics.

- [x] Create `cmd/stats.go`
- [x] Wire in `cmd/root.go`
- [x] Create formatters in `internal/format/` for stats output

Logic:
1. Parse `--since` (default: `30d`) and `--until` (default: now) via `dateutil.Parse`
2. **Lead Time**: `SearchClosedIssues(since, until)` → per-issue `metrics.LeadTime` → `ComputeStats`
3. **Cycle Time**: same issues → per-issue `cycletime.Strategy.Compute` → `ComputeStats`
4. **Throughput**: count of closed issues + `SearchMergedPRs(since, until)` count
5. **WIP**: same as `wip` command logic (board or label fallback). If neither configured → omit section with note.
6. **Quality**: find most recent release tag → run `BuildReleaseMetrics`. If no releases → omit section with note.

Graceful degradation per section:
- Each section computes independently; one failure doesn't block others
- Failed sections show `(unavailable: <reason>)` on stderr, omitted from output
- Minimum output: Lead Time + Cycle Time + Throughput (always computable)

Pretty output:
```
Stats: owner/repo (2026-02-08 – 2026-03-10 UTC)

  Lead Time:   median 3.2d, P90 8.1d (n=14)
  Cycle Time:  median 1.1d, P90 4.2d (n=14)
  Throughput:  14 issues closed, 22 PRs merged
  WIP:         7 items in progress
  Quality:     2 bugs / 8 issues (25% defect rate) — v1.2.0
```

JSON output:
```json
{
  "repository": "owner/repo",
  "window": { "since": "...", "until": "..." },
  "lead_time": { "count": 14, "median_seconds": 276480, "p90_seconds": 699840 },
  "cycle_time": { "count": 14, "median_seconds": 95040, "p90_seconds": 362880 },
  "throughput": { "issues_closed": 14, "prs_merged": 22 },
  "wip": { "count": 7, "items": [...] },
  "quality": { "release_tag": "v1.2.0", "bug_count": 2, "total_issues": 8, "defect_rate": 0.25 }
}
```

---

## Design Decisions

### `--since` overloading

`--since` means DATE on bulk commands and TAG on `quality release`/`scope`. This matches git convention (`git log --since` vs positional refs). Error messages on tag-based commands will say: `expected a tag name like v1.2.3, got "30d"`.

### Conflict: positional arg + `--since`

Error: `provide either an issue number or --since, not both`. Explicit rejection, never silent.

### Bulk results cap

Search API returns max 1000 results. If capped, warn on stderr: `results capped at 1000; narrow the date range for complete data`. Include `"capped": true` in JSON output.

### WIP age semantics

Age = time since `statusAt` (last status change). Known limitation: if item moves In Progress → Backlog → In Progress, age resets. This is acceptable — the board status is the source of truth for "current WIP duration."

### Draft issues on boards

Included in WIP output. They represent in-progress work even without a backing issue.

### Stats quality when no releases

Omit the quality section. Note on stderr: `no releases found; quality metrics unavailable`.

### Bulk sort order

Pretty/markdown: close date descending (most recent first). JSON: unsorted (API order).

## Acceptance Criteria

### Phase 1
- [x] `dateutil.Parse` handles YYYY-MM-DD, RFC3339, relative (Nd), rejects future/inverted
- [x] All timestamps in JSON output end with `Z`
- [x] All pretty/markdown timestamps show `UTC`
- [x] Table-driven tests cover all date parsing edge cases

### Phase 2
- [x] `SearchClosedIssues` returns correct issues for date range
- [x] Pagination works up to 1000 results
- [x] Warns when results are capped

### Phase 3
- [x] `lead-time --since 30d` returns per-item table + aggregate stats
- [x] `cycle-time --since 30d` returns per-item table + aggregate stats
- [x] Error when both positional arg and `--since` given
- [x] All three output formats (json, pretty, markdown) work for bulk mode
- [x] E2E test: `lead-time --since 30d -R cli/cli --config docs/examples/cli-cli.yml -f json` returns data

### Phase 4
- [x] `quality release v1.0 --since v0.9` works
- [x] `release v1.0 --since v0.9` works (deprecated alias, warns)
- [x] Smoke tests updated

### Phase 5
- [x] `wip` shows board items with status, age
- [x] `wip` falls back to label-based when no project board
- [x] `wip` errors clearly when neither source available
- [x] `wip -R owner/repo` filters cross-repo board to one repo

### Phase 6
- [x] `stats` shows 30-day dashboard with all sections
- [x] `stats --since 2026-01-01` respects custom window
- [x] Graceful degradation: missing WIP config or no releases handled
- [x] All three output formats work

## Future Considerations (Not in Scope)

- **Size weighting** — config slot designed (`size.source`, `size.weights`) but not implemented. See brainstorm section 8.
- **`quality --since DATE`** — time-based quality without release boundary. Deferred.
- **Multi-repo** — `--repos` flag or org-wide queries. Out of scope per brainstorm decision.
- **Throughput trends** — `stats --since 90d --period 30d` for month-over-month. Future.

## References

- Brainstorm: `docs/brainstorms/2026-03-10-command-hierarchy-brainstorm.md`
- Issue #9: rename release → quality release
- Cycle time signal hierarchy: `docs/solutions/cycle-time-signal-hierarchy.md`
- Event-based metrics: `docs/brainstorms/2026-03-10-event-based-metrics-brainstorm.md`
- Three-state metric pattern: `docs/solutions/three-state-metric-status-pattern.md`
- Existing search pattern: `internal/github/pullrequests.go:38` (`SearchMergedPRs`)
- Existing stats computation: `internal/metrics/metric.go` (`ComputeStats`)
