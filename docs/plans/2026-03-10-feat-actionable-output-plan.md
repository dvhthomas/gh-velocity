---
title: "feat: Actionable Output — Links, Insights, and Daily-Use Features"
type: feat
status: active
date: 2026-03-10
brainstorm: docs/brainstorms/2026-03-10-actionable-output-brainstorm.md
deepened: 2026-03-10
---

# feat: Actionable Output — Links, Insights, and Daily-Use Features

## Enhancement Summary

**Deepened on:** 2026-03-10
**Agents used:** Architecture Strategist, Performance Oracle, Security Sentinel, Code Simplicity Reviewer, Pattern Recognition Specialist, Best Practices Researcher, Framework Docs Researcher

### Key Improvements from Deepening

1. **Scope reduced ~40%** — Phases 5 (Wait-State Decomposition) and 6 (Trend Arrows) deferred to future plans. Both are data-quality-fragile and technically complex relative to value. Replaced by a "Future Candidates" section.
2. **Single `git log --numstat` approach** — Bus factor rewritten from O(D) per-directory git invocations to O(1) single-pass parsing. Critical for repos with 40+ directories.
3. **lipgloss `Hyperlink()` replaces manual OSC 8** — The indirect dependency already supports native hyperlinks with graceful degradation. No custom escape sequence code needed.
4. **Security hardening** — Input validation for git command parameters, control character sanitization for hyperlinks, privacy considerations for posted output.
5. **`RenderContext` struct** — Consolidates formatter parameters (`io.Writer`, `Format`, `IsTTY`, `Width`, `Repo`) to prevent parameter list explosion as features grow.
6. **Bot exclusion via `exclude_users` config** — REST search API supports `-author:username` natively, so bot accounts (dependabot, renovate, claude_bot) are filtered server-side with zero client-side overhead.
7. **Data quality guardrails** — Every feature that interprets GitHub data now documents what assumptions it makes and how it degrades when data is noisy or incomplete.

### Scope Changes

| Original | Decision | Reason |
|----------|----------|--------|
| Phase 5: Wait-State Decomposition | **Deferred** | Assumes clean PR lifecycle (review requested → reviewed → approved → merged). Many teams skip steps, self-merge, or use informal reviews. Data is too fragile for reliable metrics. |
| Phase 6: Trend Arrows | **Deferred** | Doubles API calls, small sample sizes make trends noisy (3 vs 5 issues = "↓40%" is meaningless). Ship when there's user demand. |
| Phase 1c: Search URLs | **Deferred** | Premature — per-item links (Phase 1a) are the high-value item. Search URLs are a separate feature if demanded. |
| `--sort` flag with 4 options | **Hardcode defaults** | Ship with sensible default sort. Add `--sort` later if users request it. |
| `--depth`, `--min-commits`, `--stale-threshold` flags | **Hardcode defaults** | Defaults acknowledged as sufficient in brainstorm. Add flags later if users hit cases where defaults are wrong. |
| Review Pressure aggregates | **Simplified** | Core value is "list PRs awaiting review." Team aggregates (median turnaround, reviews/merge) require expensive API calls for a nice-to-have. Add later. |
| `MyWeekResult.ReviewTurnaround` | **Cut** | Requires fetching PR timeline events for every reviewed PR. Scope creep. |
| `MyWeekResult.InProgress` | **Cut** | Duplicates `status wip`. Three lists is the feature. |

---

## Overview

Transform gh-velocity from an occasional metrics tool into a daily-use engineering intelligence CLI. Four phases delivering incrementally, each usable on its own. Focuses on features where GitHub data is reliable and the output is unambiguously actionable.

## Problem Statement

gh-velocity fetches rich data from GitHub (URLs, labels, timeline events) but discards most of it at render time. The tool answers "what are the numbers?" but not the questions people actually ask:

- **Developer:** "What should I tell my manager?" → writes status updates manually
- **Developer:** "What's sitting idle?" → sees WIP count but no staleness signals
- **Tech Lead:** "What PRs need review attention?" → review queue is invisible
- **Eng Leader:** "What's our bus factor?" → nobody measures it

## Data Quality Philosophy

GitHub data is only as good as the workflow hygiene of the people using it. Features in this plan are categorized by data reliability:

**Robust (ground truth data):**
- Per-item links — URLs are populated by GitHub automatically
- Bus factor — git log is ground truth; commits don't lie about who touched what
- Labels in output — showing what's there, transparently

**Reliable (GitHub tracks automatically):**
- My Week — issue/PR attribution is automatic; author is always correct
- Stale Work — `updated_at` is coarse but reliable (caveat: bot comments bump it)
- Review queue — open PRs with review requests are a concrete, queryable state

**Fragile (deferred):**
- Wait-state decomposition — assumes formal PR review lifecycle that many teams skip
- Flow efficiency — derived from wait-state data, inherits its fragility
- Trend arrows — small sample sizes make trends noisy; a "↓40%" on 3 vs 5 items is meaningless

Every feature MUST show sample sizes so users can judge reliability. A metric without context is worse than no metric.

## Proposed Solution

Four phases delivering incrementally:

1. **Infrastructure** — Links, labels, `RenderContext` (foundation for everything)
2. **My Week + Bus Factor** — Two independent, high-value features (one API, one local git)
3. **Stale Work Detector** — Enhance existing WIP with staleness signals
4. **Review Pressure** — New command surfacing the review queue

## Technical Approach

### Architecture

```
cmd/
  myweek.go           (NEW - gh velocity status my-week)
  reviews.go          (NEW - gh velocity status reviews)
  busfactor.go        (NEW - gh velocity quality bus-factor)

internal/model/
  types.go            (MODIFY - add WIP staleness fields, MyWeek result type)

internal/github/
  client.go           (MODIFY - add GetAuthenticatedUser, retry wrapper)
  search.go           (MODIFY - add author/reviewer search, extract paginatedSearch helper)

internal/git/
  git.go              (MODIFY - add ContributorsByPath with single-pass parsing)

internal/metrics/
  busfactor.go        (NEW - knowledge concentration computation)
  staleness.go        (NEW - WIP staleness scoring)

internal/format/
  formatter.go        (MODIFY - add RenderContext, link helpers)
  links.go            (NEW - lipgloss hyperlink rendering)
  myweek.go           (NEW - my-week formatters)
  reviews.go          (NEW - review pressure formatters)
  busfactor.go        (NEW - bus factor formatters)
  bulk.go             (MODIFY - add URL, labels columns)
  wip.go              (MODIFY - add staleness columns)
```

### Cross-Cutting: RenderContext

Introduced in Phase 1 to prevent formatter parameter list explosion. All formatter functions take this instead of individual `io.Writer`, `isTTY`, `width` parameters:

```go
// internal/format/formatter.go

type RenderContext struct {
    Writer  io.Writer
    Format  Format
    IsTTY   bool
    Width   int
    Owner   string   // for constructing URLs
    Repo    string
}
```

This collapses 5-6 common parameters into one struct and makes adding new output concerns (links, labels) trivial — they can read from `RenderContext` without changing every function signature.

### Cross-Cutting: Excluded Users (Bot Filtering)

Many repos have bot accounts (`@dependabot`, `@renovate`, `@claude_bot`, etc.) whose activity should be excluded from metrics and activity reports. Support an `exclude_users` config option:

```yaml
# .gh-velocity.yml
exclude_users:
  - dependabot[bot]
  - renovate[bot]
  - claude_bot
```

**Implementation:** The REST search API supports `-author:username` natively. Each excluded user becomes a `-author:{user}` qualifier appended to search queries. This is server-side filtering — zero client-side overhead.

```go
// Build exclusion string from config
func buildExclusions(users []string) string {
    var parts []string
    for _, u := range users {
        parts = append(parts, "-author:"+u)
    }
    return strings.Join(parts, " ")
}
// Appended to search queries: "is:issue is:closed -author:dependabot[bot] -author:renovate[bot]"
```

**Scope:** Applies to all search-based commands (my-week, lead-time, cycle-time, throughput, review pressure, report). Does NOT apply to bus factor (git log), where bot commits are typically rare and `--no-merges` already filters merge commits.

**Why REST search, not GraphQL search:** GraphQL's `search()` query does NOT support `-author:` negation syntax. Since bot exclusion is a core feature, stick with REST search API for all search operations. The 30 req/min REST throttle is manageable with the retry wrapper (see below).

### Cross-Cutting: Search API Strategy

**Use REST search with retry wrapper.** Although GraphQL search avoids the 30 req/min throttle, it doesn't support `-author:` negation needed for bot exclusion. REST search with retry-on-429 is the better choice.

**Wire `rateLimitWait` into a retry wrapper.** The existing function in `internal/github/ratelimit.go` is dead code — no caller uses it. Before adding more API calls, create a retry wrapper:

```go
func (c *Client) doRESTWithRetry(ctx context.Context, method, path string, body, resp interface{}) error {
    for attempt := 0; attempt < 3; attempt++ {
        err := c.rest.DoWithContext(ctx, method, path, body, resp)
        if err == nil { return nil }
        wait, isRateLimit := rateLimitWait(err)
        if !isRateLimit { return err }
        select {
        case <-time.After(wait):
        case <-ctx.Done(): return ctx.Err()
        }
    }
    return fmt.Errorf("rate limited after 3 attempts")
}
```

**Extract `paginatedSearch` helper** before adding new search functions. The existing `SearchClosedIssues` and `SearchMergedPRs` duplicate the same pagination loop. Extract it once, apply retry, and all new search functions become two-liners.

### Implementation Phases

#### Phase 1: Infrastructure — Links, Labels, RenderContext

Foundation that all subsequent features depend on. High impact, low risk — surfaces data already being fetched and prevents parameter explosion.

##### 1a. Per-Item Links

**model changes:** None — `URL` fields already exist on `Issue`, `PR`, `Commit`, `Release`.

**`internal/format/links.go` (NEW):**

```go
// FormatItemLink renders an issue/PR reference with link appropriate to format.
// Uses lipgloss Hyperlink() for pretty/TTY, full URL markdown links for markdown,
// and passes URL through to JSON structs.
func FormatItemLink(number int, url string, rc RenderContext) string

// FormatReleaseLink renders a release tag with link.
func FormatReleaseLink(tag, url string, rc RenderContext) string
```

**lipgloss native hyperlinks (no manual OSC 8):**

```go
// internal/format/links.go

func FormatItemLink(number int, url string, rc RenderContext) string {
    text := fmt.Sprintf("#%d", number)
    switch rc.Format {
    case FormatMarkdown:
        return fmt.Sprintf("[%s](%s)", text, url)
    case FormatJSON:
        return text // URL goes in separate JSON field
    default: // Pretty
        if !rc.IsTTY || url == "" {
            return text
        }
        // lipgloss handles OSC 8 and graceful degradation automatically
        return lipgloss.NewStyle().
            Underline(true).
            Hyperlink(url).
            Render(text)
    }
}
```

**Security: sanitize control characters** in any user-controlled text passed to hyperlink rendering. Issue numbers (integers) and GitHub-generated URLs are safe, but as defense-in-depth:

```go
// stripControlChars removes bytes 0x00-0x1f and 0x7f from text.
// Prevents terminal escape sequence injection if untrusted text
// is ever passed to hyperlink rendering.
func stripControlChars(s string) string
```

**JSON struct changes in `internal/format/json.go`:**

Add `url` field to all item-level JSON structs: `JSONIssueMetrics`, `JSONBulkLeadItem`, `JSONCycleTimeOutput`, `jsonItem`, `JSONWIPItem`. URLs are already populated on model types — just pass them through.

**Modify every formatter** that renders issue/PR numbers to use `FormatItemLink`. Adopt `RenderContext` at the same time. Files: `bulk.go`, `pretty.go`, `markdown.go`, `wip.go`, `scope.go`, `cycletime.go`.

##### 1b. Labels in Output

**Modify item-listing formatters** to include labels:
- Pretty: comma-separated, truncated by tableprinter
- Markdown: comma-separated
- JSON: `"labels": ["bug", "priority:high"]`

Labels are already on `model.Issue.Labels` and `model.PR.Labels`. No new API calls needed.

**Not added to:** single-item views, aggregate-only views, report summary.

##### 1c. Model Preparation

Add `UpdatedAt time.Time` to `model.Issue` and `model.WIPItem` now (Phase 1), even though staleness computation comes in Phase 3. This avoids shipping infrastructure that references a field that doesn't exist yet. The GitHub API returns `updated_at` — capture it in `internal/github/issues.go` and `search.go`.

##### Phase 1 Acceptance Criteria

- [ ] All item-listing commands show clickable hyperlinks (lipgloss in TTY, plain text in non-TTY)
- [ ] Markdown output uses `[#42](url)` for all issue/PR references
- [ ] JSON output includes `url` field on all item-level structs
- [ ] Labels shown in bulk lead-time, bulk cycle-time, WIP, release item tables
- [ ] `RenderContext` adopted across all existing formatters
- [ ] `paginatedSearch` helper extracted; `rateLimitWait` wired into retry wrapper
- [ ] `UpdatedAt` captured on `model.Issue` and `model.WIPItem`
- [ ] `stripControlChars` applied in `FormatItemLink`
- [ ] All existing tests pass; new table-driven tests for links

---

#### Phase 2: My Week + Bus Factor (parallel, independent)

Two high-value features that can be built simultaneously. My Week needs API; Bus Factor needs local git. No dependency between them.

##### 2a. My Week — Developer's 1:1 Prep Tool

**New command:** `gh velocity status my-week` (under `status` group, consistent with command hierarchy)

**Flags:**
- `--format` (pretty/markdown/json) — inherited from root
- `--since` — optional override, defaults to rolling 7 days from current UTC time

**Identity resolution (`internal/github/client.go`):**

```go
// GetAuthenticatedUser returns the login of the authenticated GitHub user.
// Uses GET /user endpoint. Works with all token types (classic PAT, fine-grained PAT, GitHub App).
// Result cached on Deps.AuthenticatedUser for the session.
func (c *Client) GetAuthenticatedUser(ctx context.Context) (string, error)
```

Cache on `Deps.AuthenticatedUser string` (not a separate context key — consistent with existing `Deps` pattern).

**Data fetching (`internal/github/search.go` — modify):**

```go
// New search functions — all use the extracted paginatedSearch helper.
// Naming convention: consistently use "ByAuthor" suffix.
func (c *Client) SearchClosedIssuesByAuthor(ctx context.Context, login string, since, until time.Time) ([]model.Issue, error)
func (c *Client) SearchMergedPRsByAuthor(ctx context.Context, login string, since, until time.Time) ([]model.PR, error)
func (c *Client) SearchReviewedPRsByAuthor(ctx context.Context, login string, since, until time.Time) ([]model.PR, error)
```

Search queries (REST search — supports `-author:` exclusion for bot filtering):
- Issues closed by user: `is:issue is:closed author:{login} closed:{since}..{until} repo:{owner}/{repo} -author:dependabot[bot] ...`
- PRs merged by user: `is:pr is:merged author:{login} merged:{since}..{until} repo:{owner}/{repo} -author:...`
- PRs reviewed by user: `is:pr reviewed-by:{login} updated:{since}..{until} repo:{owner}/{repo}`

All search queries append `-author:{user}` for each entry in `config.ExcludeUsers`.

**Model (`internal/model/types.go`):**

```go
type MyWeekResult struct {
    Login        string
    Since        time.Time    // NOT a pre-formatted string — format in format/ package
    Until        time.Time
    IssuesClosed []Issue
    PRsMerged    []PR
    PRsReviewed  []PR
}
```

Note: `Period string` replaced with `Since`/`Until time.Time` for consistency with `StatsResult`, `ThroughputResult`, and all other time-windowed result types. Formatting is the formatter's job.

**Formatters (`internal/format/myweek.go` — NEW):**

```go
func WriteMyWeekPretty(rc RenderContext, result model.MyWeekResult) error
func WriteMyWeekMarkdown(rc RenderContext, result model.MyWeekResult) error
func WriteMyWeekJSON(rc RenderContext, result model.MyWeekResult) error
```

- Pretty: sections with headers, item tables with links
- Markdown: sections with `##` headers, linked items, designed for paste-into-Slack/doc
- JSON: full `MyWeekResult` serialization with URLs

**Persona boundary:** No `--user` or `--author` flag. Always uses authenticated user. Error if `gh auth status` fails.

**Edge cases:**
- GitHub Actions: `GetAuthenticatedUser` returns the bot/PAT user. Warn: "Authenticated as [bot-name]. my-week shows activity for the authenticated user."
- No activity in period: "No activity in the last 7 days" — success, not error.
- Search API caps at 1000 results per query. Document this limitation.

**Data quality:** My Week is reliable — GitHub automatically tracks issue/PR authorship. The `reviewed-by:` qualifier may miss informal reviews done via comments rather than formal GitHub review submissions.

##### 2b. Bus Factor — Knowledge Risk from Local Git

**New command:** `gh velocity quality bus-factor`

**Flags:**
- `--format` (pretty/markdown/json) — inherited
- `--since` — defaults to `90d` (90 days)

No `--depth` or `--min-commits` flags — hardcode depth=2 and min-commits=5. Add flags later only if users hit cases where defaults are wrong.

**Requires local git checkout.** Check `deps.HasLocalRepo` (consistent with existing pattern in `root.go`). If false, error with: `"bus-factor requires a local git checkout. Run from within the repository or use a GitHub Action with actions/checkout."`

**Security: validate `--since` input.** The `--since` value MUST go through `dateutil.Parse()` and be reformatted as an ISO date string before being passed to any `exec.CommandContext` call. Never pass raw flag values to git commands.

```go
// In cmd/busfactor.go RunE:
sinceTime, err := dateutil.Parse(sinceFlag)
if err != nil {
    return model.NewAppError("invalid --since value: "+sinceFlag, model.ExitConfig)
}
sinceStr := sinceTime.Format("2006-01-02") // safe for git --since=
```

**Git operations — SINGLE PASS approach (`internal/git/git.go`):**

```go
// ContributorsByPath runs a single git log with --numstat and aggregates
// contributor data per directory path, truncated to the specified depth.
// This is O(1) process spawns instead of O(D) per-directory invocations.
func (r *Runner) ContributorsByPath(ctx context.Context, since time.Time, depth int, minCommits int) ([]PathContributors, error)
```

Implementation: ONE git command, parsed in a single streaming pass:

```go
// Single command — NOT per-directory
args := []string{"log",
    "--format=%H%x00%aN%x00%aE",
    "--numstat",
    "--no-merges",
    "--since=" + since.Format("2006-01-02"),
}
```

Parse output: each commit block is `hash\0name\0email`, followed by blank line, followed by `added\tremoved\tfilepath` lines, followed by blank line. Truncate each filepath to `depth` directory levels, aggregate `map[directory]map[email]commitCount` in memory.

**Why single-pass matters:** Per-directory `git log` is O(D) process spawns. A repo with 40 directories at depth 2 takes 4+ seconds. A monorepo with 200+ directories takes 60+ seconds. Single-pass is always <5 seconds regardless of directory count.

**Metrics (`internal/metrics/busfactor.go` — NEW):**

```go
type BusFactorResult struct {
    Paths []PathRisk
    Since time.Time
    Depth int
}

type PathRisk struct {
    Path             string
    Risk             RiskLevel
    ContributorCount int
    Primary          git.Contributor
    PrimaryPct       float64
    TotalCommits     int
}

type RiskLevel string
const (
    RiskHigh   RiskLevel = "HIGH"    // 1 contributor
    RiskMedium RiskLevel = "MEDIUM"  // 2 contributors, primary >70%
    RiskLow    RiskLevel = "LOW"     // 3+ contributors, distributed
)
```

**Formatters (`internal/format/busfactor.go` — NEW):**

```go
func WriteBusFactorPretty(rc RenderContext, result metrics.BusFactorResult) error
func WriteBusFactorMarkdown(rc RenderContext, result metrics.BusFactorResult) error
func WriteBusFactorJSON(rc RenderContext, result metrics.BusFactorResult) error
```

Pretty output (with lipgloss colors for risk levels):
```
Knowledge Risk Report (last 90 days, depth 2)

Risk   Path                    Contributors   Primary          Commits
HIGH   internal/strategy/      1              alice (100%)     47
MEDIUM internal/github/        2              bob (78%)        156
LOW    internal/metrics/       4              distributed      89
LOW    cmd/                    3              distributed      62

Summary: 1 HIGH risk, 1 MEDIUM risk, 2 LOW risk areas
```

**Privacy:** Pretty and markdown output show contributor names only (from `%aN`). Email addresses included only in JSON output where they are useful for programmatic identity resolution.

**Edge cases:**
- Solo developer: all paths show HIGH risk. This is correct and useful.
- `.mailmap` not present: `%aE` still works; multiple emails for same person = overcounted contributors (false LOW risk). Document this.

**Data quality:** Bus factor is robust — git log is ground truth. The only caveat is `.mailmap` for identity normalization, which is the standard git solution and well-understood.

##### Phase 2 Acceptance Criteria

- [ ] `gh velocity status my-week` shows issues closed, PRs merged, PRs reviewed with links
- [ ] `gh velocity status my-week --format markdown` produces paste-ready output
- [ ] My-week uses authenticated user identity (cached on `Deps`), errors on auth failure
- [ ] `gh velocity quality bus-factor` shows per-directory knowledge risk
- [ ] Bus factor uses single `git log --numstat` invocation (verify with `--debug`)
- [ ] Bus factor completes in <5s for repos with <10,000 commits
- [ ] Bus factor validates `--since` through `dateutil.Parse()` before passing to git
- [ ] Bus factor checks `deps.HasLocalRepo`, errors clearly when unavailable
- [ ] Privacy: contributor emails only in JSON output, names only in pretty/markdown
- [ ] All three output formats work for both commands

---

#### Phase 3: Stale Work Detector

Enhances the existing `status wip` command with staleness signals. Moderate scope — modifies existing code rather than adding new commands.

**Model changes (`internal/model/types.go`):**

```go
type WIPItem struct {
    Number    int
    Title     string
    Status    string
    Age       time.Duration
    Repo      string
    Kind      string
    URL       string           // NEW — from issue/PR URL (added in Phase 1)
    Labels    []string         // NEW — from issue/PR labels (added in Phase 1)
    UpdatedAt time.Time        // NEW — added in Phase 1c
    Staleness StalenessLevel   // NEW — computed signal
}

// StalenessLevel — NOT "StalenessSignal" to avoid collision with existing
// Signal* constants (SignalIssueCreated, SignalIssueClosed, etc.) which
// represent lifecycle event signals.
type StalenessLevel string
const (
    StalenessActive StalenessLevel = "ACTIVE"   // activity within 3 days
    StalenessAging  StalenessLevel = "AGING"     // 3-7 days since activity
    StalenessStale  StalenessLevel = "STALE"     // >7 days since activity
)
```

Note: renamed from `StalenessSignal` with `Signal*` prefix constants to `StalenessLevel` with `Staleness*` prefix to avoid namespace collision with existing lifecycle signal constants.

**Data source:** `UpdatedAt` comes from the GitHub Issue/PR `updated_at` field (already added to model in Phase 1c). When fetching WIP items via project board (`ListProjectItems`), verify the GraphQL query returns `updatedAt` on the content nodes. If not, add a batch fetch for issue details.

Note: `ProjectItem` also needs a `URL` field, or the WIP command needs an additional fetch to get issue/PR URLs. Verify the GraphQL query in `ListProjectItems` returns the content URL.

**Staleness computation (`internal/metrics/staleness.go` — NEW):**

```go
func ComputeStaleness(updatedAt time.Time, now time.Time) StalenessLevel
```

Thresholds (hardcoded — no `--stale-threshold` flag):
- ACTIVE: updated within 3 days
- AGING: updated 3-7 days ago
- STALE: updated >7 days ago

**Formatter changes (`internal/format/wip.go`):**

Pretty output (with lipgloss colors — text labels carry meaning for colorblind accessibility):
```
#  Title                    Status        Age    Last Activity    Signal
87 Payment refactor         In Progress   23d    18d ago          STALE
52 API v2 migration         In Review     12d    11d ago          STALE
93 Add dark mode            In Progress    8d    2d ago           AGING
71 Fix login flow           In Progress    3d    1d ago           ACTIVE
```

- Pretty: lipgloss colored text — STALE (red), AGING (yellow), ACTIVE (green)
- Markdown: same table, text labels only (no color)
- JSON: `"staleness": "STALE"`, `"updated_at": "2026-02-20T..."`, `"last_activity_days": 18`

Default sort: age descending (oldest first) — preserves current behavior.

**Data quality:** `updated_at` is reliable but coarse. Bot comments, label changes, and automated actions all bump it, which can make items appear "ACTIVE" when no human has touched them. This is a known limitation — document it. Users will quickly learn to interpret the signals in context.

##### Phase 3 Acceptance Criteria

- [ ] `status wip` shows "Last Activity" column and staleness signal
- [ ] Staleness levels are ACTIVE/AGING/STALE with correct thresholds
- [ ] Pretty format uses lipgloss colored text for levels
- [ ] All three output formats include staleness data
- [ ] `ProjectItem` URL is resolved and appears in WIP output

---

#### Phase 4: Review Pressure

New command surfacing the review queue. Simplified from the original plan — focuses on listing PRs awaiting review rather than computing team aggregates.

**New command:** `gh velocity status reviews`

**Flags:**
- `--format` (inherited)
- `--since` — defaults to `7d` (used for aggregate context, not filtering the queue)

**API calls:**

```go
// internal/github/search.go — add to existing file, not a new file

// SearchOpenPRsAwaitingReview finds open PRs with pending review requests.
// Uses c.owner/c.repo (NOT parameters — consistent with all other search methods).
func (c *Client) SearchOpenPRsAwaitingReview(ctx context.Context) ([]model.PR, error)
// Query: is:pr is:open review:required repo:{owner}/{repo}
// Fallback: is:pr is:open — filter client-side for PRs with review requests
```

Note: `review:required` is a valid GitHub search qualifier (confirmed in docs research). If it's unavailable on certain GitHub plans, fall back to `is:pr is:open` with client-side filtering.

**Model (`internal/model/types.go`):**

```go
type ReviewPressureResult struct {
    AwaitingReview []PRAwaitingReview
}

type PRAwaitingReview struct {
    Number  int
    Title   string
    URL     string
    Age     time.Duration   // since PR was opened or review was requested
    IsStale bool            // >48h without review (hardcoded threshold)
}
```

Simplified from original: no `TotalReviews`, `MedianTurnaround`, `ReviewsPerMerge`, or `StalePRs` count. The stale count is derived at render time (`len(filter(...))`). Team aggregates require expensive `FetchPRReviews` GraphQL calls — add later if demanded.

**Formatters (`internal/format/reviews.go` — NEW):**

```go
func WriteReviewsPretty(rc RenderContext, result model.ReviewPressureResult) error
func WriteReviewsMarkdown(rc RenderContext, result model.ReviewPressureResult) error
func WriteReviewsJSON(rc RenderContext, result model.ReviewPressureResult) error
```

Pretty output:
```
Review Queue: owner/repo

PRs Awaiting Review (sorted by wait time):
  #142  Add export feature          3d 12h   STALE
  #138  Fix payment flow            1d 4h
  #145  Update docs                 6h

3 PRs awaiting review (1 stale >48h)
```

**Persona boundary:** Shows PR titles and ages — no reviewer names. For teams of 2, the list is still shown — it's about the *work* (PRs), not the *people*.

**Edge cases:**
- No open PRs awaiting review: "No PRs currently awaiting review." — success, not error.
- Solo contributor: "Solo repository — no review data." (review pressure is meaningless without multiple contributors)

**Data quality:** Review queue is reliable — open PRs with review requests are a concrete, queryable state in GitHub. The main caveat is teams that don't use GitHub's formal review request mechanism (they just @ mention in comments). For those teams, this command will show an empty queue, which is accurate for the data available.

##### Phase 4 Acceptance Criteria

- [ ] `gh velocity status reviews` shows PRs awaiting review sorted by wait time
- [ ] PRs waiting >48h are flagged as STALE
- [ ] No individual reviewer names or rankings appear in output
- [ ] All three output formats work
- [ ] Solo repository detected and handled gracefully

---

## Future Candidates (Deferred)

These features are in the product vision but deferred due to data quality concerns, technical complexity, or lack of proven user demand. Each can be planned separately when there's signal.

### Wait-State Decomposition

Break cycle time into coding/waiting-review/in-review/waiting-merge segments with visual bars. **Deferred because:** assumes a clean PR lifecycle that many teams don't follow. Self-merges, informal reviews, and skipped review requests produce misleading or empty segments. Requires new PR timeline API surface (GraphQL `timelineItems` with `REVIEW_REQUESTED_EVENT`, `PULL_REQUEST_REVIEW`). See brainstorm for full design.

**Prerequisites for unblocking:** User feedback confirming teams use formal GitHub review workflows. Prototype with a single repo to validate data quality before building the full feature.

### Flow Efficiency Score

Active work time / total elapsed time as a single percentage. **Deferred because:** derived from wait-state decomposition data — inherits all its fragility. A "18% flow efficiency" number based on incomplete timeline events is worse than no number.

### Trend Arrows

Compare every metric to the previous equivalent period with ↑/↓/→ indicators. **Deferred because:** doubles API calls (current + previous period), small sample sizes make percentage changes noisy (3 vs 5 items = "↓40%" is meaningless), and the `--trend` flag adds complexity for a feature nobody has asked for yet.

**When to reconsider:** If users request period-over-period comparison, implement with named periods (week/month/quarter), rate-normalize partial periods for throughput metrics, and require minimum sample sizes before showing trend arrows.

### Scope Creep Detector

Compare planned vs. shipped items per release. **Deferred because:** the baseline mechanism ("what was planned?") needs design work. Requires milestones, project boards, or another signal to define the release scope.

### Search URLs

Contextual GitHub search URLs appended to output. **Deferred because:** per-item links (Phase 1a) provide the high-value click-through. Search URLs are a separate feature if users want "explore this set" functionality.

### `--sort` Flag

Multiple sort options (age, updated, duration, closed) on bulk commands. **Deferred because:** hardcoded defaults are sufficient for v1. Each command sorts by the most useful dimension. Add `--sort` later if users request alternative orderings.

---

## Alternative Approaches Considered

1. **Interactive TUI (rejected):** Violates "print and exit" philosophy. CLI is for quick answers, not extended sessions.
2. **Formatter interface refactor (deferred):** `RenderContext` struct is a lighter-weight solution that prevents parameter explosion without a full interface. Revisit if the free-function pattern becomes painful.
3. **GitHub Actions as separate tool (rejected):** CLI already works in Actions via `gh extension exec`. Single codebase is simpler.
4. **Manual OSC 8 escape sequences (rejected):** lipgloss v1.1.x already has native `Hyperlink()` support with graceful degradation. No custom escape code needed.
5. **Per-directory `git log` for bus factor (rejected):** O(D) process spawns is unacceptably slow. Single `git log --numstat` with in-memory aggregation is O(1) process spawns.
6. **GraphQL search API (rejected for search operations):** Avoids the 30 req/min REST throttle, but does NOT support `-author:` negation syntax needed for bot exclusion. REST search with retry wrapper is the better choice since bot filtering is a core requirement.

## Acceptance Criteria

### Functional Requirements

- [ ] All 4 features produce output in pretty, markdown, and JSON formats
- [ ] Per-item links render correctly in all formats (lipgloss hyperlink in TTY, plain text in non-TTY, full URL in markdown, `url` field in JSON)
- [ ] My-week is strictly self-serve (no `--user` flag)
- [ ] Bus factor works offline with zero API calls
- [ ] No individual developer rankings or comparisons in any output
- [ ] Every metric shows sample size so users can judge reliability
- [ ] `exclude_users` config option filters bot accounts from all search-based commands
- [ ] Bot exclusion uses REST search `-author:` syntax (server-side, not client-side filtering)

### Non-Functional Requirements

- [ ] No new direct dependencies beyond promoting lipgloss from indirect to direct
- [ ] Bus factor completes in <5s for repos with <10,000 commits (single git invocation)
- [ ] `--since` values validated through `dateutil.Parse()` before passing to git commands
- [ ] Control characters sanitized in hyperlink rendering
- [ ] `rateLimitWait` wired into retry wrapper for all API calls

### Quality Gates

- [ ] Table-driven tests for all new metric computations
- [ ] Integration tests for new commands via `task build` + binary execution
- [ ] Existing smoke tests continue to pass
- [ ] `task quality` passes (lint + staticcheck)

## Dependencies & Prerequisites

- **lipgloss** — already an indirect dependency via go-gh v2.13.0 (v1.1.x). Promote to direct dependency. The current pinned version supports `Hyperlink()` and `table` sub-package. Pin version carefully to avoid conflicts with go-gh's transitive dependency.
- **`GET /user` endpoint** — needed for my-week identity. Works with all token types including fine-grained PATs with zero permissions.
- **Resolve `buildClosingPRMap` duplication** — exists in both `cmd/helpers.go` and `internal/metrics/dashboard.go`. Consolidate before adding more API orchestration code.

## Risk Analysis & Mitigation

| Risk | Severity | Mitigation |
|------|----------|------------|
| Git command injection via `--since` | HIGH | Validate through `dateutil.Parse()`, format as ISO date before passing to git |
| Terminal escape injection via hyperlinks | MEDIUM | `stripControlChars()` on all text passed to lipgloss `Hyperlink()` |
| Search API 30 req/min throttle | MEDIUM | Use GraphQL search where possible; add `rateLimitWait` retry wrapper |
| Privacy leak via PR titles in posted markdown | MEDIUM | Document that `--post` and `--format markdown` expose titles from private repos |
| Git author identity ambiguity in bus factor | LOW | Use `.mailmap`-resolved email (`%aE`); document limitation |
| lipgloss version conflict with go-gh | LOW | Pin version to match go-gh's transitive dependency |
| `updated_at` coarseness for staleness | LOW | Document that bot activity bumps `updated_at`; users learn to interpret in context |

## References & Research

### Internal References

- Brainstorm: `docs/brainstorms/2026-03-10-actionable-output-brainstorm.md`
- Model types: `internal/model/types.go`
- Formatter pattern: `internal/format/formatter.go`
- GraphQL timeline pattern: `internal/github/cyclestart.go:GetClosingPR`
- Batched GraphQL pattern: `internal/github/pullrequests.go:FetchPRLinkedIssues`
- TablePrinter wrapper: `internal/format/table.go`
- Dashboard computation: `internal/metrics/dashboard.go:ComputeDashboard`
- Rate limit utility (dead code): `internal/github/ratelimit.go`
- Signal hierarchy learning: `docs/solutions/cycle-time-signal-hierarchy.md`
- TablePrinter learning: `docs/solutions/go-gh-tableprinter-migration.md`
- Three-state metric learning: `docs/solutions/three-state-metric-status-pattern.md`

### External References

- lipgloss Hyperlink() support: github.com/charmbracelet/lipgloss (v1.1.x+)
- lipgloss/table package: pkg.go.dev/github.com/charmbracelet/lipgloss/table
- OSC 8 adoption tracker: github.com/Alhadis/OSC8-Adoption
- GitHub Search API qualifiers: docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests
- GitHub GraphQL rate limits: docs.github.com/en/graphql/overview/rate-limits-and-query-limits-for-the-graphql-api
- GitHub REST /user endpoint: docs.github.com/en/rest/users/users
- GitHub GraphQL PullRequest timelineItems: docs.github.com/en/graphql/reference/objects
- DORA metrics and flow efficiency: dora.dev/guides/dora-metrics/
