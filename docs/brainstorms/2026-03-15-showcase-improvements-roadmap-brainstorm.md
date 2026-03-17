# Showcase Improvements Roadmap

**Date:** 2026-03-15
**Status:** Brainstorm

## What We're Building

A three-phase improvement roadmap driven by findings from the daily velocity showcase (Discussion #64). The showcase revealed that even moderately active repos hammer the GitHub API, the output lacks actionable insights, and the posting model needs richer configuration.

## Phase 1: Disk Cache + N+1 Fix (API Optimization)

### Problem

Running `report` + 4 individual commands for a single repo makes ~30-60 API calls. Each command is a separate process with no shared state — the same search query is fetched 5 times. The showcase hit GitHub's secondary rate limit repeatedly, with 1-minute backoff penalties making a 2-repo run take 25 minutes.

Additionally, cycle time with issue strategy has an N+1 GraphQL problem — `GetProjectStatus` makes one call per issue, uncached.

### Approach: Short-TTL disk cache + N+1 batching

- **Disk cache** at `~/.cache/gh-velocity/` (or `$XDG_CACHE_HOME/gh-velocity/`)
- Keyed by SHA256 of the query string (same as existing in-memory cache)
- Short TTL (~5 minutes) — long enough for a batch of commands, short enough to never serve stale data in normal use
- Internal-only format — marshalled data in whatever format is convenient (gob, JSON, etc.), not a user contract. Can change between versions without notice.
- `--no-cache` flag to bypass (for debugging, testing)
- Auto-cleanup: expired entries removed on startup or via background goroutine
- No user should ever rely on the cache; it's purely an internal optimization to avoid redundant API calls

**N+1 fix:** Batch `GetProjectStatus` calls using GraphQL aliases (fetch up to 50 issues' project status in one query, similar to the existing `FetchPRLinkedIssues` pattern).

### Expected Impact

- Showcase: 5x reduction in API calls per repo (report caches, individual commands reuse)
- Interactive use: running `lead-time` then `cycle-time` for the same window reuses cached search results
- N+1 fix: cycle time with issue strategy goes from N+1 to ceil(N/50)+1 GraphQL calls

## Phase 2: Richer Output (Insights + Links)

### Problem

Command output shows raw stats without interpretation. Users see "median 3.2d, P90 8h" but don't know if that's good, bad, or skewed by outliers. The debug output shows working GitHub search URLs that users never see.

Related issues: #65, #66, #67

### Approach: Insights engine + link generation in all formats

**Insights (every command, all formats):**
- Add IQR (Q1-Q3) to `model.Stats` computation
- Flag outliers (items > 1.5x IQR above Q3) with specific issue/PR references
- Generate narrative insights: "most work takes X-Y hours, but 2 items were unusually slow"
- Pretty: narrative text. Markdown: narrative + links. JSON: structured insight objects.

**Links (markdown format):**
- Scope search links — already computed by `scope.Query.URL()`, just surface in output
- Issue/PR references as clickable links (`https://github.com/{owner}/{repo}/issues/{number}`)
- Must be tested — URL encoding is tricky

**Multi-format output (#67):**
- `--format` accepts a comma-separated list: `--format md,json`
- When multiple formats specified, `--output <basename>` is required (no extension)
- Produces `<basename>.md` and `<basename>.json` (extension derived from format)
- Single format without `--output` works as today (stdout)
- Single pipeline run, multiple output sinks
- Example: `gh velocity report --since 30d --format md,json --output velocity-report`

### Expected Impact

- Users get actionable findings without needing to interpret raw stats
- Clickable links make reports navigable
- Dual output halves API usage for CI pipelines that need both formats

## Phase 3: Posting Configuration

### Problem

Today, `--post` can only post to a Discussion category on the repo being analyzed. The showcase needs to post cli/cli results to dvhthomas/gh-velocity — requiring shell orchestration with `gh api graphql`. Every user who wants similar behavior would need to script it themselves.

### Approach: Full posting config in `.gh-velocity.yml`

```yaml
discussions:
  repo: dvhthomas/gh-velocity          # target repo (default: analyzed repo)
  category: Velocity Reports            # existing field, now with repo context
  title: "Velocity: {{.Repo}} ({{.Date}})"  # Go template, convention default
  mode: new-discussion                  # new-discussion | update-body | add-comment
```

**Convention over configuration:**
- `repo`: defaults to the repo being analyzed (current behavior)
- `category`: required when posting (current behavior)
- `title`: defaults to `"gh-velocity {{.Command}}: {{.Repo}} ({{.Date}})"`
- `mode`: defaults to `update-body` (idempotent upsert, current behavior)
- `add-comment` mode requires adding `AddDiscussionComment` GraphQL mutation to the Go code

### Expected Impact

- Showcase can use `--post` directly instead of shell orchestration
- Any user can configure cross-repo posting in their config
- The shell script in `scripts/showcase.sh` becomes much simpler (just loop + `--post`)

## Key Decisions

1. **Priority order:** Cache → Output → Posting (fix the API pain first, then improve value, then improve delivery)
2. **Disk cache is internal-only:** opaque format, no user contract, `--no-cache` to bypass, auto-cleanup
3. **N+1 fix bundled with cache:** natural pairing since both are in the API layer
4. **Insights in ALL formats:** pretty, markdown, and JSON all get insights (not just markdown)
5. **Posting config is full-featured:** repo + category + title template + mode, with sensible defaults
6. **Dual output via `--tee-json`:** single run, two output sinks

## Resolved Questions

1. **Cache TTL:** 5 minutes — long enough for command batches, short enough to never serve stale data
2. **Cache location:** `~/.cache/gh-velocity/` or `$XDG_CACHE_HOME/gh-velocity/`
3. **Cache format:** Internal/opaque — can change between versions. Not a user contract.
4. **Implementation order:** Cache first (unblocks showcase at scale), then output (increases value), then posting (improves delivery)

## Resolved Questions (Phase 2)

5. **Cache versioning:** Version in cache dir path (`~/.cache/gh-velocity/v1/`). New version = fresh cache. No compatibility concerns.
6. **Insights mode:** Always on. If it's valuable data, show it. No opt-in flag needed.
7. **Comment markers for add-comment mode:** Reuse existing `<!-- gh-velocity:command:context -->` marker pattern. Proven, consistent, already tested.
8. **Dual output design:** `--format md,json --output <basename>` produces `<basename>.md` and `<basename>.json`. Single format without `--output` works as today (stdout only). Multiple formats require `--output`.

## Open Questions

None — all questions resolved.
