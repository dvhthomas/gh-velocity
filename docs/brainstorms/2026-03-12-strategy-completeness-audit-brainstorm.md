---
title: "Strategy Completeness Audit: Do All Config Combinations Produce Meaningful Results?"
date: 2026-03-12
type: research
---

# Strategy Completeness Audit

## Executive Summary

A thorough audit of all subcommands × config combinations reveals that gh-velocity works well for **PR-centric repos** and **repos with project boards**, but has significant gaps for the most common case: **issue-centric repos without a project board**. The default `strategy: issue` produces N/A cycle time silently unless a project board is configured — and most repos don't have one. This creates a "first run disappointment" problem.

---

## Command × Strategy Matrix

### What each command actually needs to produce value

| Command | Scope | Categories | Strategy | Lifecycle | Project Board | Notes |
|---------|:-----:|:----------:|:--------:|:---------:|:-------------:|-------|
| `flow lead-time` (single) | — | — | — | — | — | Always works (created→closed) |
| `flow lead-time --since` | ✓ | — | — | — | — | Always works |
| `flow cycle-time` (single) | — | — | **critical** | **critical** | **if issue** | N/A without signal |
| `flow cycle-time --since` | ✓ | — | **critical** | **critical** | **if issue** | N/A without signal |
| `flow cycle-time --pr` | — | — | — | — | — | Always works (PR created→merged) |
| `flow throughput` | ✓ | — | — | — | — | Always works |
| `status my-week` | ✓ | — | **used** | **used** | **if issue** | Works, cycle time may be N/A |
| `status reviews` | — | — | — | — | — | Always works |
| `status wip` | — | — | — | — | **required** | Not yet implemented |
| `quality release` | — | **critical** | **used** | **if issue** | **if issue** | Works, cycle time may be N/A |
| `quality bus-factor` | — | — | — | — | — | Always works (local git) |
| `report` | ✓ | optional | **used** | **used** | **if issue** | Partial: sections that fail are omitted |

### The five realistic config scenarios

#### Scenario 1: Minimal config (preflight --write, no project board, no issue types)
```yaml
scope:
  query: "repo:owner/repo"
quality:
  categories:
    - name: bug
      match: ["label:bug"]
    - name: feature
      match: ["label:enhancement"]
cycle_time:
  strategy: issue   # default
```

| Command | Result | User Value |
|---------|--------|-----------|
| lead-time | ✅ Works | High — always has data |
| cycle-time (single) | ⚠️ N/A (silent) | **None** — no lifecycle signal |
| cycle-time (bulk) | ⚠️ All items N/A | **None** — no warning why |
| throughput | ✅ Works | High |
| my-week | ⚠️ No cycle time | Medium — lead time + counts work |
| release | ⚠️ No cycle time | Medium — categories + lead time work |
| report | ⚠️ Cycle time section empty | Medium |

**Problem**: This is the most common scenario and cycle time is silently useless.

#### Scenario 2: PR strategy (preflight --write, more PRs than issues)
```yaml
scope:
  query: "repo:owner/repo"
quality:
  categories:
    - name: bug
      match: ["label:bug"]
cycle_time:
  strategy: pr
```

| Command | Result | User Value |
|---------|--------|-----------|
| lead-time | ✅ Works | High |
| cycle-time (single issue) | ⚠️ Depends on closing PR | Medium — N/A if no PR links issue |
| cycle-time (bulk) | ✅ Works for issues with closing PRs | High |
| cycle-time --pr | ✅ Works | High |
| throughput | ✅ Works | High |
| my-week | ✅ Cycle time from merged PRs | High |
| release | ✅ Cycle time from linked PRs | High |
| report | ✅ Works | High |

**Verdict**: PR strategy works well across the board when repos close issues via PRs.

#### Scenario 3: Issue strategy with project board (preflight --write --project-url ...)
```yaml
scope:
  query: "repo:owner/repo"
quality:
  categories: [...]
cycle_time:
  strategy: issue
project:
  url: "https://github.com/users/me/projects/1"
  status_field: "Status"
lifecycle:
  backlog:
    project_status: ["Backlog"]
  in-progress:
    project_status: ["In progress"]
  done:
    project_status: ["Done"]
```

| Command | Result | User Value |
|---------|--------|-----------|
| lead-time | ✅ Works | High |
| cycle-time (single) | ✅ Real cycle time | **High** |
| cycle-time (bulk) | ✅ Real cycle time (N+1 API calls) | High (slow for large windows) |
| throughput | ✅ Works | High |
| my-week | ✅ Real cycle time | **High** |
| release | ✅ Real cycle time per issue | **High** |
| report | ✅ All sections work | **High** |

**Verdict**: Best experience. All commands produce full value.

#### Scenario 4: Issue strategy, lifecycle configured but project resolution fails
Same as Scenario 3 config, but project URL is wrong or PAT lacks `project` scope.

| Command | Result | User Value |
|---------|--------|-----------|
| cycle-time | ⚠️ Warning logged, then N/A | Low — warning only in logs |
| Everything else | Same as Scenario 1 | Medium |

**Problem**: Warning is logged to stderr but easily missed. User sees N/A with no explanation in output.

#### Scenario 5: No config at all
| Command | Result |
|---------|--------|
| All commands | ❌ Error: "no config found" with helpful next steps |

**Verdict**: Good error guidance — directs to preflight.

---

## Critical Findings

### Finding 1: The "Silent N/A" Problem

**Severity: High**

When `strategy: issue` is configured without a project board (Scenario 1 — the default), cycle time is silently N/A across ALL consumers. No warning appears in command output. The warning only fires during `buildCycleTimeStrategy` if project resolution fails, but if there's simply no lifecycle config, nothing is logged.

**Impact**: Users run preflight, get a config, run `cycle-time --since 30d`, and see every item as "N/A" with no explanation. They don't know if the tool is broken or if they need more config.

**Fix options**:
1. **Emit a warning when issue strategy produces zero results**: "cycle time unavailable: no lifecycle.in-progress.project_status configured. See docs/guide.md or run preflight with --project-url"
2. **Preflight should suggest PR strategy** when no project board is available (currently only suggests PR when PRs > issues)
3. **Default strategy should be "pr"** when no project board — it's more likely to produce useful data
4. **Show "N/A (no lifecycle signal)" instead of bare "N/A"** in output

### Finding 2: Preflight Strategy Heuristic is Too Conservative

**Severity: High**

Current heuristic: `strategy = "pr"` only when `recentPRs > recentIssues`. Otherwise `strategy = "issue"`.

**Problem**: Issue strategy without a project board is useless for cycle time. Preflight should be smarter:
- If project board exists → `strategy: issue` (correct)
- If no project board AND repo has PRs closing issues → `strategy: pr` (better default)
- If no project board AND no PRs → `strategy: issue` with prominent warning

**Proposed heuristic**:
```
if hasProjectBoard && hasLifecycleInProgress:
    strategy = "issue"    # best signal available
elif recentPRsMerged > 0:
    strategy = "pr"       # PRs are available, use them
else:
    strategy = "issue"    # fallback, but warn prominently
```

### Finding 3: lifecycle.done.query is Never Used

**Severity: Medium**

The `lifecycle.done.query` config field exists but is never consumed by any command. All date-range commands hardcode `is:closed` / `is:merged` via the scope helpers (`ClosedIssueQuery`, `MergedPRQuery`). The lifecycle query values in config are dead config.

**Impact**: Users (and preflight) generate lifecycle.done.query thinking it's used, but it's ignored. Only `lifecycle.*.project_status` is actually consumed (by cycle time's project board detection).

**Fix options**:
1. Remove `query` from lifecycle stages (breaking change, confusing)
2. Document that `query` is reserved for future use
3. Actually wire lifecycle queries into scope assembly (adds complexity)
4. At minimum, don't generate `lifecycle.done.query` in preflight since it misleads

### Finding 4: N+1 API Problem for Issue Strategy in Bulk Mode

**Severity: Medium**

When issue strategy is configured with a project board, every issue needs a separate `GetProjectStatus` GraphQL call. For bulk mode with 100+ issues, this means 100+ sequential API calls. PR strategy avoids this with pre-fetching (`BuildClosingPRMap`).

**Impact**: `cycle-time --since 30d` on an active repo could take minutes. `my-week` could be slow for prolific users.

**Current mitigation**: `errgroup.SetLimit(5)` provides some concurrency. But this is still O(N) API calls.

**Fix options** (future):
1. Batch query: GraphQL aliases to query multiple issues' project items in one call
2. Accept N+1 for now, document the tradeoff

### Finding 5: "docs" Category Excluded from Preflight Generation

**Severity: Low**

Preflight iterates only `["bug", "feature", "chore"]` when generating category config. The "docs" category is detected in labels but not included in the generated config. Users who want docs tracking must add it manually.

**Fix**: Add "docs" to the preflight category generation loop.

### Finding 6: Report Quality Section Hardcodes "bug" as Defect

**Severity: Low**

`computeQuality()` in `cmd/report.go` counts only issues classified as "bug" for the defect rate. Users who name their bug category differently (e.g., "defect", "regression") get a 0% defect rate.

**Fix options**:
1. Use a configurable "defect categories" list
2. Use the first category as the defect indicator (convention-based)
3. Document that "bug" is the magic name for defect rate

### Finding 7: Empty Results Have No Explanation

**Severity: High**

When any bulk command returns 0 results, the output simply shows an empty table or "0 issues, 0 PRs." There's no guidance about why: wrong scope? wrong date range? no activity? The user is left to guess.

**Fix**: When search returns 0 results, print a hint:
- "No issues found. Check your scope (currently: `repo:owner/repo`) and date range (since: X, until: Y)."
- Include the GitHub search URL so users can verify manually
- Some commands already include `search_url` in JSON output — surface it in pretty/markdown too

### Finding 8: WIP Command is Not Implemented

**Severity: Low** (known)

`status wip` exists as a command but returns "not yet supported." The TODO references PRs C and D. This is a known gap but should be documented in command help as "coming soon" rather than erroring at runtime.

---

## Preflight-to-Command Success Matrix

Does `preflight --write` produce a config that gives meaningful results for each command?

| Scenario | lead-time | cycle-time | throughput | my-week | release | report |
|----------|:---------:|:----------:|:----------:|:-------:|:-------:|:------:|
| No project, few PRs | ✅ | ❌ N/A | ✅ | ⚠️ partial | ⚠️ partial | ⚠️ partial |
| No project, many PRs | ✅ | ✅ pr | ✅ | ✅ | ✅ | ✅ |
| With project board | ✅ | ✅ issue | ✅ | ✅ | ✅ | ✅ |
| Project URL wrong/no scope | ✅ | ❌ N/A | ✅ | ⚠️ partial | ⚠️ partial | ⚠️ partial |

**The critical gap**: The first row (most common new-user scenario) produces a disappointing first experience for cycle-time-related features.

---

## Recommended Improvements (Prioritized)

### P0: Fix the First-Run Experience

1. **Change preflight strategy heuristic**: Default to `strategy: pr` when no project board is available and repo has any merged PRs. Issue strategy without a project board is a guaranteed N/A.

2. **Add prominent hints in preflight output** when issue strategy is chosen without lifecycle.in-progress:
   ```
   ⚠ Cycle time will be unavailable with strategy "issue" — no lifecycle.in-progress.project_status configured.
     To enable cycle time:
       • Add a project board: preflight --project-url <url>
       • Switch to PR strategy: set cycle_time.strategy to "pr" in your config
   ```

3. **Surface N/A reason in command output**: Change "N/A" to "N/A (no lifecycle signal)" in cycle-time output.

### P1: Improve Runtime Guidance

4. **Empty results hint**: When bulk commands return 0 results, show scope + date range + search URL so users can debug.

5. **Aggregate cycle-time warnings**: In bulk mode, emit one summary warning: "Cycle time unavailable for N of M issues (no lifecycle signal)" instead of per-item silence.

6. **Include search URLs in pretty/markdown output** (already in JSON) so users can verify what GitHub actually returns.

### P2: Config Accuracy

7. **Stop generating lifecycle.done.query**: It's never consumed. Either wire it into scope assembly or stop generating it in preflight/templates.

8. **Add "docs" to preflight category generation** — it's detected but not included.

9. **Document that "bug" is the magic category name** for report defect rate. Or make it configurable.

### P3: Documentation

10. **Add "No results?" troubleshooting section** to docs/guide.md covering: wrong scope, wrong date range, no activity, missing lifecycle config.

11. **Add strategy decision guide**: A clear flowchart for "which strategy should I pick?" based on repo workflow.

12. **Document lifecycle.query vs lifecycle.project_status** distinction clearly — users are confused about which does what.

13. **Document the N+1 performance characteristic** of issue strategy in bulk mode so users know what to expect.

---

## Sources

- `cmd/cycletime.go`, `cmd/report.go`, `cmd/myweek.go`, `cmd/release.go` — all cycle time consumers
- `cmd/preflight.go` — strategy heuristic (lines 295-303), lifecycle mapping (lines 930-957)
- `cmd/helpers.go` — `buildCycleTimeStrategy` (lines 14-35)
- `internal/metrics/cycletime.go` — IssueStrategy.Compute (lines 40-70)
- `internal/scope/scope.go` — query assembly helpers
- `internal/config/config.go` — lifecycle config structure
- `internal/format/` — rendering layer for all output formats
- `docs/guide.md`, `README.md`, `docs/examples/` — documentation
