---
title: "feat: gh velocity CLI extension"
type: feat
status: completed
date: 2026-03-09
brainstorm: docs/brainstorms/2026-03-09-gh-velocity-brainstorm.md
deepened: 2026-03-09
---

## Enhancement Summary

**Deepened on:** 2026-03-09
**Sections enhanced:** Architecture, API Strategy, Config, Error Handling, Implementation Phases, Acceptance Criteria
**Review agents used:** Architecture Strategist, Performance Oracle, Security Sentinel, Code Simplicity Reviewer, Pattern Recognition Specialist, Agent-Native Reviewer, Best-Practices Researcher

### Key Improvements
1. **Architecture:** Added `internal/git/` (local git operations) and `internal/model/` (shared domain types) packages; narrow role-specific interfaces instead of monolithic `Client` interface
2. **Posting safety:** `--dry-run` flag for `--post`; idempotent posting with HTML comment markers to detect and update existing posts
3. **Error discipline:** Structured JSON errors with exit code table (0=success, 1=general, 2=config, 3=auth, 4=not found); input validation on all args
4. **API efficiency:** Batch GraphQL calls via `nodes()` queries; bounded concurrency with `errgroup`; `context.Context` propagated from root command
5. **Security:** GraphQL variables only (never string interpolation); validate all user-supplied numeric args with `strconv.Atoi`; no secrets in error messages
6. **Phase 1 vertical slice:** Build `release` command first as end-to-end proof, then extract shared patterns for other commands

### Triage Decisions (2026-03-09)
- **Subcommands:** Keep named by *output metric* (lead-time, cycle-time, release, etc.), not data source. Commands stay separate.
- **Formats:** 3 for v1: JSON, pretty, markdown. CSV deferred.
- **Workflow:** Remove auto-detect. Default to `pr` mode; users set `workflow: local` in config.
- **Packages:** Keep `internal/model/` and `internal/git/`.
- **Integration tests:** Must run against a locally built binary (via `task build`), not `go run`.

### Future Considerations
- **DORA Metrics Module:** Full DORA metric support (Deployment Frequency, Lead Time for Changes, Change Failure Rate, MTTR) deferred to a future version. See [github-dora-metrics](https://github.com/mikaelvesavuori/github-dora-metrics) and [lead-time-for-changes](https://github.com/DeveloperMetrics/lead-time-for-changes) for reference implementations. DORA metrics require GitHub Actions deployment event data and incident tracking beyond what v1's issue/commit/release model provides. Current metrics are *informed by* DORA but are not strict DORA implementations.

---

# gh velocity: GitHub Velocity & Quality Metrics CLI Extension

## Overview

A Go-based GitHub CLI extension that computes established velocity and quality metrics from GitHub data (issues, commits, releases, Projects v2) and posts them where the work happens — issue comments, discussions, and release notes.

Evolves the bash metrics scripts from [go-calcmark](https://github.com/CalcMark/go-calcmark) into a testable, composable, agent-friendly CLI tool.

## Problem Statement

Engineering velocity and quality data lives locked inside commercial dashboards or requires manual calculation. gh-velocity makes this data:
- **Computable** from data already in GitHub (issues, commits, tags, Projects v2)
- **Postable** directly on issues, discussions, and releases
- **Consumable** by agents (JSON), spreadsheets (CSV), and humans (pretty/markdown)
- **Transparent** with auditable, configurable calculations based on DORA and established metrics

## Technical Approach

### Architecture

```
gh-velocity (binary)
├── main.go                          # Entry point, calls cmd.Execute()
├── cmd/                             # Cobra commands (one file per subcommand)
│   ├── root.go                      # Root command, global flags, config loading
│   ├── leadtime.go                  # gh velocity lead-time <issue>
│   ├── cycletime.go                 # gh velocity cycle-time <issue>
│   ├── summary.go                   # gh velocity summary <issue>
│   ├── prmetrics.go                 # gh velocity pr-metrics <pr>
│   ├── release.go                   # gh velocity release <tag>
│   ├── throughput.go                # gh velocity throughput
│   ├── project.go                   # gh velocity project
│   └── version.go                   # gh velocity version
├── internal/
│   ├── config/                      # .gh-velocity.yml parsing and validation
│   │   ├── config.go
│   │   └── config_test.go
│   ├── model/                       # Shared domain types (Issue, PR, Release, Commit)
│   │   └── types.go                 # Pure structs, no API dependency
│   ├── github/                      # GitHub API client abstraction
│   │   ├── client.go                # Wraps go-gh REST + GraphQL clients
│   │   ├── client_test.go
│   │   ├── issues.go                # Issue queries
│   │   ├── commits.go               # Commit/tag queries
│   │   ├── releases.go              # Release queries
│   │   ├── pullrequests.go          # PR queries
│   │   └── projects.go              # Projects v2 GraphQL queries
│   ├── git/                         # Local git operations (tags, commit ranges)
│   │   ├── git.go                   # exec.Command wrappers
│   │   └── git_test.go
│   ├── metrics/                     # Metric computation (pure logic, no API calls)
│   │   ├── leadtime.go              # Lead time calculation
│   │   ├── leadtime_test.go
│   │   ├── cycletime.go             # Cycle time calculation
│   │   ├── cycletime_test.go
│   │   ├── quality.go               # Change failure rate, escaped defects, release composition
│   │   ├── quality_test.go
│   │   ├── rework.go                # Rework rate
│   │   ├── rework_test.go
│   │   ├── planvsactual.go          # Plan vs actual dates
│   │   ├── planvsactual_test.go
│   │   ├── stats.go                 # Mean, median, sample SD
│   │   └── stats_test.go
│   ├── linking/                     # Commit-to-issue linking heuristics
│   │   ├── linker.go                # Multi-source issue reference discovery
│   │   └── linker_test.go
│   ├── format/                      # Output formatters
│   │   ├── formatter.go             # Formatter interface
│   │   ├── json.go                  # JSON output
│   │   ├── markdown.go              # Markdown table output (also used for --post)
│   │   ├── pretty.go                # TTY-aware table output (go-gh tableprinter)
│   │   └── formatter_test.go
│   └── posting/                     # GitHub posting (issue comments, discussions, releases)
│       ├── poster.go                # Post dispatcher
│       └── poster_test.go
├── .gh-velocity.yml                 # Example config (used for testing)
├── Taskfile.yaml                    # Build, test, lint, quality tasks
├── .golangci.yml                    # Linter config
├── .goreleaser.yaml                 # Release config
├── go.mod
├── go.sum
├── CLAUDE.md                        # Development workflow for agents
└── README.md
```

#### Architecture Research Insights

**Package boundaries:**
- `internal/model/` holds shared domain types (`Issue`, `PR`, `Release`, `Commit`) as plain structs with no API dependency. Both `github/` and `metrics/` import `model/`, but `metrics/` never imports `github/`.
- `internal/git/` isolates local git operations (`exec.Command` for `git log`, `git tag`) from the API client. This keeps `github/` focused on API calls and makes git operations independently testable with a test repo fixture.

**Narrow interfaces over monolithic client:**
Instead of one large `Client interface` with 15+ methods, define role-specific interfaces consumed where needed:

```go
// In internal/github/
type IssueQuerier interface {
    GetIssue(ctx context.Context, number int) (*model.Issue, error)
    ListIssueEvents(ctx context.Context, number int) ([]model.Event, error)
}

type ReleaseQuerier interface {
    ListReleases(ctx context.Context) ([]model.Release, error)
    GetRelease(ctx context.Context, tag string) (*model.Release, error)
}
```

This follows Go's interface segregation principle — consumers declare what they need, the concrete `Client` struct satisfies all of them. Tests mock only the methods they use.

**Context propagation:** Pass `context.Context` from Cobra's `cmd.Context()` through every API call and git operation. This enables cancellation (Ctrl+C) and future timeout support without refactoring.

### Key Dependencies

| Dependency | Purpose | Why |
|-----------|---------|-----|
| `github.com/cli/go-gh/v2` | Auth, REST client, GraphQL client, tableprinter, repo context | Official gh extension library. Handles auth automatically. |
| `github.com/spf13/cobra` | CLI framework | Standard for Go CLIs. Used by gh itself. |
| `gopkg.in/yaml.v3` | Config parsing | Parse `.gh-velocity.yml` |

**Not using:**
- `go-github`: go-gh's REST client is sufficient for a gh extension and handles auth automatically. go-github adds typed methods but requires separate auth setup.
- `shurcooL/githubv4`: go-gh's GraphQL client supports struct-based and string-based queries. Direct githubv4 dependency unnecessary.

### API Strategy

### Two Clocks: Work Completion vs. Release Delivery

The tool tracks two distinct timelines:

1. **Work completion** — measured by `lead-time`, `cycle-time`, `summary`
   - Lead time: issue created → work complete (Done/Closed)
   - Cycle time: first commit → work complete
   - Quality gates (reviews, tests) live here
   - Independent of release strategy

2. **Release delivery** — measured by `release`, `throughput`
   - Release cadence: how often do we ship? (descriptive, not prescriptive)
   - Release composition: what % is bugs vs features?
   - Release quality: did this release cause follow-up bug fixes?
   - Release lag: time from work-complete to included-in-release

This separation means a team's work velocity isn't penalized by a deliberate monthly release cadence, and a team that ships every commit isn't artificially rewarded on lead time.

- **REST** (issues, PRs, commits, releases): `go-gh/v2/pkg/api.DefaultRESTClient()` — auto-authenticated, JSON responses decoded into structs
- **GraphQL** (Projects v2): `go-gh/v2/pkg/api.DefaultGraphQLClient()` — string-based `Do()` for complex queries with inline fragments
- **Git operations** (tags, commit ranges): Local `git` commands via `exec.Command` — faster than API, works offline for tag/commit data
- **Auth**: Fully delegated to go-gh. No manual token management. For Projects v2, detect missing `project` scope reactively and print: `error: Projects v2 requires the 'project' scope. Run: gh auth refresh --scopes project`

#### API Strategy Research Insights

**GraphQL security:** Always use GraphQL variables for user-supplied values. Never interpolate strings into queries.

```go
// CORRECT — variables
variables := map[string]interface{}{
    "owner": owner,
    "name":  repo,
    "number": issueNumber,
}
err := client.Do(query, variables, &result)

// WRONG — string interpolation (injection risk)
query := fmt.Sprintf(`{ repository(owner: "%s", name: "%s") { ... } }`, owner, repo)
```

**Batching with `nodes()`:** When fetching multiple issues by ID, use GraphQL `nodes()` to batch into a single request instead of N individual calls:

```graphql
query($ids: [ID!]!) {
  nodes(ids: $ids) {
    ... on Issue { number title closedAt }
  }
}
```

**Bounded concurrency:** For operations requiring multiple API calls (e.g., per-issue metrics in `release`), use `errgroup` with a semaphore:

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(5) // max 5 concurrent API calls
for _, issue := range issues {
    g.Go(func() error {
        return fetchIssueMetrics(ctx, issue)
    })
}
return g.Wait()
```

**Rate limit handling:** Check `x-ratelimit-remaining` header on REST responses. For GraphQL, check the `rateLimit` field in the response. Back off *before* hitting the limit rather than retrying after 403.

### Config System

`.gh-velocity.yml` lives at the repo root. Config is **optional** — every field has a sensible default. Subcommands that need specific config fields fail with a clear error if those fields are missing.

```yaml
# All fields optional. Shown with defaults.
workflow: pr            # "local" or "pr" (default: pr)

project:                # Required only for `project` and `plan-vs-actual` in `summary`
  id: "PVT_xxx"
  status_field_id: "PVTSSF_xxx"

statuses:               # Maps conceptual states to project board values
  backlog: "Backlog"
  ready: "Ready"
  in_progress: "In progress"
  in_review: "In review"
  done: "Done"

fields:                 # Optional Projects v2 custom field names
  start_date: "Start Date"
  target_date: "Target Date"

quality:
  bug_labels: ["bug"]                # Labels that identify bug issues
  feature_labels: ["enhancement"]    # Labels that identify feature issues
  hotfix_window_hours: 72            # Patch release within this window = hotfix

discussions:
  category_id: ""       # GraphQL node ID for discussion category (for release posts)
```

**Config required per subcommand:**

| Subcommand | Required Config | Optional Config |
|-----------|----------------|-----------------|
| `lead-time` | none | `workflow` |
| `cycle-time` | none | `workflow` |
| `summary` | none | `workflow`, `project` (for plan-vs-actual), `quality.bug_labels` (for rework) |
| `pr-metrics` | none | none |
| `release` | none | `quality` (for release composition), `discussions` (for `--post`) |
| `throughput` | none | `quality.bug_labels`, `quality.feature_labels` (for type breakdown) |
| `project` | `project.id` | `statuses`, `fields`, `quality` |

**Validation:** Fail hard on malformed YAML. Ignore unknown keys (forward-compatible). Type errors (e.g., `hotfix_window_hours: "abc"`) produce a clear error with the field name and expected type.

#### Config System Research Insights

**Non-finite float guard (from go-calcmark learning):** If any config field accepts numeric values from YAML, guard against `NaN`/`Inf` before use. YAML 1.1 permits `.nan`, `.inf`, and `-.inf` as valid floats, which can cause panics in downstream math. Apply `math.IsNaN`/`math.IsInf` checks on any `float64` from untrusted YAML input.

**Config validation strategy:** Validate config eagerly at load time (in `PersistentPreRunE`), not lazily at use time. This catches errors early with clear context. Store the validated config on the Cobra command context so subcommands access it without re-loading.

### Subcommands

#### `gh velocity lead-time <issue>`
Computes: Issue created → work complete (issue moved to Done/Closed, or last commit/PR merged).
This measures **work completion**, not release delivery. The time from work-complete to release is **release lag**, tracked separately in the `release` command.
Falls back to "now" if issue is still open.

#### `gh velocity cycle-time <issue>`
- **local mode** (`workflow: local`): First commit → last commit referencing the issue
- **pr mode** (`workflow: pr`, default): First commit → PR merged (for the PR that closes the issue)
Measures active development time. The distinction from lead time: lead time includes queue/wait time before work starts.

#### `gh velocity summary <issue>`
All applicable metrics for one issue: lead time, cycle time, rework (reopened count), plan-vs-actual (if Projects v2 fields exist). One-stop view.

#### `gh velocity pr-metrics <pr>`
PR size (additions, deletions, changed files, commits), time to first review, review-to-merge time, comment count.

#### `gh velocity release <tag>`
The flagship command. Measures **delivery** separately from work completion:
- Per-issue: lead time, cycle time, commit count, **release lag** (work complete → release tag date)
- Aggregates: mean, median, sample SD
- Release composition: bug ratio, feature ratio, other ratio — tells the quality story
- Release cadence: time since previous release
- Quality: change failure rate (is this a hotfix release?), escaped defects count

Release cadence is a business decision, not inherently good/bad. A CLI tool releasing monthly and a SaaS deploying daily are both valid — the tool reports, doesn't judge.

**Previous tag detection:** Tags sorted by semver. Non-semver tags sorted by date. `--since <tag>` flag to override. First release: diff from initial commit.

#### `gh velocity throughput`
Issues closed and PRs merged per period, with type breakdown (bugs, features, other).
- `--from` / `--to` date flags (default: last 30 days)
- `--group-by week|month` for trend view

#### `gh velocity project`
Velocity and quality metrics across all issues in a GitHub Projects v2 board.
- `--from` / `--to` date flags (filter by issue close date)
- Requires `project.id` in config

#### `gh velocity version`
Prints version, built with ldflags.

### Global Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--format` | `-f` | `pretty` | Output format: `json`, `pretty`, `markdown` |
| `--post` | | false | Post output to GitHub (target depends on subcommand) |
| `--repo` | `-R` | auto-detect | Repository in `owner/name` format |

### `--post` Target Mapping

| Subcommand | Post Target | Notes |
|-----------|-------------|-------|
| `lead-time <issue>` | Comment on issue | |
| `cycle-time <issue>` | Comment on issue | |
| `summary <issue>` | Comment on issue | |
| `pr-metrics <pr>` | Comment on PR | |
| `release <tag>` | Discussion (using `discussions.category_id`) | Falls back to updating GitHub Release body if no category configured |
| `throughput` | Requires `--discussion <id>` or `--issue <number>` | No implicit target |
| `project` | Requires `--discussion <id>` or `--issue <number>` | No implicit target |

**`--post` behavior:**
- Writes to both stdout AND GitHub (not exclusive)
- `--post` always uses markdown format for the GitHub target, regardless of `--format`
- `--format` controls stdout output only

#### Posting Research Insights

**`--dry-run` flag:** Add `--dry-run` that works with `--post`. When set, print the markdown that *would* be posted to stderr without actually posting. Low-cost safety net.

**Idempotent posting with HTML comment markers:** When posting to an issue comment or discussion, embed an invisible HTML comment marker (e.g., `<!-- gh-velocity:lead-time:42 -->`) in the posted content. On subsequent `--post` runs, search for existing comments with that marker and *update* the comment instead of creating a duplicate. This makes repeated runs safe and keeps discussions clean.

```go
// Marker format: <!-- gh-velocity:{subcommand}:{identifier} -->
marker := fmt.Sprintf("<!-- gh-velocity:%s:%s -->", subcommand, identifier)
```

**Agent-native consideration:** Agents calling `gh velocity ... --post` repeatedly should never create duplicate comments. The idempotent posting pattern handles this automatically.

### Output Format Specifications

**Duration formatting rules:**
- < 1 minute: `42s`
- < 1 hour: `28m`
- < 1 day: `10h 43m`
- ≥ 1 day: `3d 13h`
- Zero: `0s`
- Negative (retroactive issue): show as negative with warning in stderr

**Machine-readable durations:**
- JSON: integer seconds (`"cycle_time_seconds": 1543`)
- Pretty/Markdown: human-readable strings

**Empty/degenerate results:**
- No results: empty table with explanatory message
- Single item: mean = that value, median = that value, SD = `--` (undefined for N=1)
- N/A metrics (e.g., no commits found): `N/A` in pretty/markdown, `null` in JSON
- Warnings printed to stderr, exit code 0

**Aggregate statistics:**
- Sample standard deviation (N-1 denominator)
- `--` displayed when N < 2

### Quality Metrics: Release Composition

For each release, classify issues by label:
- **Bug ratio**: issues with bug labels / total issues
- **Feature ratio**: issues with feature labels / total issues
- **Other ratio**: remaining issues / total issues

This tells a story: a release with 80% bugs suggests the previous release had quality problems. A pattern of alternating feature→bugfix releases is a quality signal.

**Change Failure Rate:** % of releases that are hotfixes (semver patch within `hotfix_window_hours` of a minor/major). Calculated over a configurable window (`--releases N`, default: last 10).

**Escaped Defects:** Bug-labeled issues created between release X and release X+1, attributed to release X.

### Commit-to-Issue Linking

Hardcoded heuristics (matching existing bash scripts):

1. `#N` or `(#N)` in commit message — note: this matches both issues and PRs; tool resolves the type via API
2. Closing keywords: `fixes #N`, `closes #N`, `resolves #N` (case-insensitive)
3. PR-to-issue links: if a commit is part of a PR, check the PR's closing issues
4. Commit SHAs mentioned in issue body/comments (reverse lookup)

For squash merges: parse the PR body for issue references (the individual commit messages are in the squash commit body).

### Error Handling

| Scenario | Behavior |
|----------|----------|
| No `.gh-velocity.yml` | Proceed with defaults. Warn if a subcommand needs config fields that are missing. |
| Malformed YAML | Fail with parse error and line number |
| Missing `project` scope | `error: Projects v2 requires the 'project' scope. Run: gh auth refresh --scopes project` |
| Rate limited (REST) | Retry with backoff, respect `Retry-After` header. Max 3 retries. |
| Rate limited (GraphQL) | Check `x-ratelimit-remaining`, back off before hitting limit. |
| Issue/PR not found | `error: issue #N not found in owner/repo` |
| No tags in repo | `lead-time` uses "now" with note. `release` fails with helpful message. Quality metrics unavailable. |
| Non-semver tags | Quality metrics (hotfix detection) unavailable with warning. Velocity metrics still work (tag order by date). |
| Run outside git repo | Require `--repo` flag. Error: `error: not a git repository. Use --repo owner/name.` |

#### Error Handling Research Insights

**Exit code table:**

| Code | Meaning | Example |
|------|---------|---------|
| 0 | Success | Normal output |
| 1 | General error | API failure, unexpected error |
| 2 | Config error | Malformed YAML, missing required field |
| 3 | Auth error | Missing scope, expired token |
| 4 | Not found | Issue/PR/tag doesn't exist |

**Structured JSON errors:** When `--format json` is set, errors should also be JSON:

```json
{"error": "issue #999 not found in owner/repo", "code": 4}
```

This lets agents parse errors programmatically instead of scraping stderr.

**Input validation:** Validate all user-supplied arguments early in `RunE`:

```go
issueNumber, err := strconv.Atoi(args[0])
if err != nil {
    return fmt.Errorf("invalid issue number %q: must be a positive integer", args[0])
}
if issueNumber <= 0 {
    return fmt.Errorf("invalid issue number %d: must be a positive integer", issueNumber)
}
```

**Security:** Never include tokens, auth headers, or full API responses in error messages. Sanitize errors before display.

## Implementation Phases

### Phase 1: Foundation & Vertical Slice

Split into two sub-phases. Phase 1a gets the skeleton working end-to-end with the most complex command (`release`). Phase 1b extracts shared patterns for simpler commands.

#### Phase 1a: Scaffold + `release` as vertical slice

**Goal:** Working `release` command that exercises config, API, metrics, linking, and formatting end-to-end.

**Rationale:** `release` touches every internal package (config, github, git, linking, metrics, format). Building it first proves the architecture and reveals integration issues early. Simpler commands (`lead-time`, `cycle-time`) then reuse established patterns.

**Tasks:**

- [x] Scaffold with `gh extension create --precompiled=go gh-velocity`
- [x] Restructure into `main.go` + `cmd/` + `internal/` layout
- [x] Set up Taskfile.yaml with `build`, `test`, `lint`, `quality` tasks
- [x] Set up `.golangci.yml` (port from go-calcmark)
- [x] Create CLAUDE.md with development workflow
- [x] Initialize `internal/model/types.go` — shared domain types (`Issue`, `Commit`, `Release`, `PR`)
- [x] Initialize `internal/config/` — YAML parsing, validation, defaults
  - `config.go`: `Load()`, `Config` struct, field defaults
  - `config_test.go`: table-driven tests for valid, malformed, missing, partial configs
- [x] Initialize `internal/github/client.go` — go-gh REST + GraphQL client wrapper
  - Role-specific interfaces (e.g., `IssueQuerier`, `ReleaseQuerier`) instead of monolithic `Client`
  - `client_test.go`: mock with `httptest.NewServer`
- [x] Initialize `internal/git/git.go` — local git operations (`git log`, `git tag`) via `exec.CommandContext`
  - `git_test.go`: tests with a temp git repo fixture
- [x] Implement `internal/github/issues.go` — fetch issue by number (created date, labels, state, events)
- [x] Implement `internal/github/commits.go` — commits between tags, commit search by message pattern
- [x] Implement `internal/github/releases.go` — list releases/tags, find release containing commit
- [x] Implement `internal/linking/linker.go` — commit-to-issue linking heuristics
  - `linker_test.go`: table-driven tests for each heuristic pattern
- [x] Implement `internal/metrics/leadtime.go` — lead time calculation (pure function, takes timestamps)
  - `leadtime_test.go`: table-driven tests including edge cases (no release, negative, zero)
- [x] Implement `internal/metrics/cycletime.go` — cycle time calculation
  - `cycletime_test.go`: table-driven tests for local mode and pr mode
- [x] Implement `internal/metrics/quality.go` — change failure rate, escaped defects, release composition
  - `quality_test.go`: table-driven tests for hotfix detection, release composition ratios
- [x] Implement `internal/metrics/stats.go` — mean, median, sample SD
  - `stats_test.go`: edge cases (N=0, N=1, N=2, large N)
- [x] Implement `internal/format/formatter.go` — `Formatter` interface
- [x] Implement `internal/format/json.go` — JSON output (durations as seconds)
- [x] Implement `internal/format/markdown.go` — markdown table output (also used for `--post`)
- [x] Implement `internal/format/pretty.go` — TTY-aware table (go-gh tableprinter)
- [x] Implement `cmd/root.go` — root command, `--format`, `--repo`, `--dry-run` flags, config loading in `PersistentPreRunE`, `context.Context` propagation
- [x] Implement `cmd/release.go` — `gh velocity release <tag>` (full vertical slice)
- [x] Implement `cmd/version.go` — version with ldflags

**Success criteria:**
- `gh velocity release v1.0.0` produces a metrics table with per-issue lead time, cycle time, commit count, release lag, aggregates, and release composition — in all 4 formats
- All metric calculations have table-driven tests
- `task test` and `task quality` pass

#### Phase 1b: Core commands (reuse patterns from release)

**Goal:** Working `lead-time` and `cycle-time` commands, reusing patterns established by `release`.

**Tasks:**

- [x] Implement `cmd/leadtime.go` — `gh velocity lead-time <issue>`
- [x] Implement `cmd/cycletime.go` — `gh velocity cycle-time <issue>`
- [x] Input validation: `strconv.Atoi` on issue number arg, positive integer check

**Success criteria:**
- `gh velocity lead-time 42` prints lead time in all 4 formats
- `gh velocity cycle-time 42` works in both local and pr mode
- Input validation rejects non-numeric and negative args with clear errors

### Phase 2: Summary & PR Commands

**Goal:** Working `summary` and `pr-metrics` commands. (Note: `release` was built in Phase 1a.)

**Tasks:**

- [ ] Implement `internal/github/pullrequests.go` — PR data (size, reviews, merge time)
- [ ] Implement `internal/metrics/rework.go` — rework rate (reopened events)
  - `rework_test.go`
- [ ] Implement `cmd/summary.go` — `gh velocity summary <issue>`
  - Combines lead time, cycle time, rework, PR metrics if applicable
- [ ] Implement `cmd/prmetrics.go` — `gh velocity pr-metrics <pr>`
  - Size, review time, review depth

**Success criteria:**
- `gh velocity summary 42` shows all applicable metrics
- `gh velocity pr-metrics 15` shows PR size and review data

### Phase 3: Throughput, Project & Posting

**Goal:** Working `throughput` and `project` commands. `--post` flag.

**Tasks:**

- [ ] Implement `internal/github/projects.go` — Projects v2 GraphQL queries
  - Fetch project items with field values (status, dates)
  - Cursor-based pagination
  - `project` scope detection (catch 403, print actionable error)
- [ ] Implement `internal/metrics/planvsactual.go` — plan vs actual calculation
  - `planvsactual_test.go`
- [ ] Implement `cmd/throughput.go` — `gh velocity throughput`
  - `--from` / `--to` flags (default: last 30 days)
  - `--group-by week|month` flag
  - Type breakdown by labels
- [ ] Implement `cmd/project.go` — `gh velocity project`
  - `--from` / `--to` date filtering
  - Requires `project.id` in config
- [ ] Implement `internal/posting/poster.go` — post to GitHub
  - Issue/PR comments via REST
  - Discussions via GraphQL (create discussion mutation)
  - Release body update via REST
  - Always markdown format for posted content
  - Idempotent posting: embed `<!-- gh-velocity:{cmd}:{id} -->` marker, update existing comment if found
- [ ] Add `--post` flag to all subcommands (wired in `cmd/root.go`)
- [ ] `--dry-run` with `--post`: print what would be posted to stderr, don't post
- [ ] Add `--discussion` and `--issue` flags for `throughput` and `project` posting targets

**Success criteria:**
- `gh velocity throughput --from 2026-01-01 --to 2026-03-01 --group-by month` works
- `gh velocity project --from 2026-01-01` works with Projects v2
- `gh velocity release v1.0.0 --post` creates a discussion
- `gh velocity summary 42 --post` adds a comment to issue #42

### Phase 4: Polish & Release

**Goal:** Release-ready extension.

**Tasks:**

- [ ] Set up `.goreleaser.yaml` for cross-platform builds
- [ ] Set up GitHub Actions workflow for CI (test, lint, quality on push)
- [ ] Set up release workflow (triggered by tag push, uses goreleaser)
- [ ] Add shell completion support (cobra built-in)
- [ ] Write README.md with usage examples, config reference, metric definitions
- [ ] Test with real repos: go-calcmark, gh-velocity itself
- [ ] Handle GitHub Enterprise (`GH_HOST` support via go-gh — should work automatically)

**Success criteria:**
- `gh extension install owner/gh-velocity` works
- CI passes on all platforms
- README covers all subcommands with examples
- All DORA-aligned metrics are documented with definitions

## Acceptance Criteria

### Functional Requirements

- [ ] All 7 subcommands produce correct output in all 3 formats (JSON, pretty, markdown)
- [ ] `--post` flag writes to the correct GitHub target per subcommand
- [ ] `.gh-velocity.yml` config is parsed with sensible defaults; missing config doesn't crash
- [ ] Workflow mode (`pr` default, `local` override) correctly controls cycle time calculation
- [ ] Commit-to-issue linking finds references via message patterns, PR links, and closing keywords
- [ ] Release composition shows bug/feature/other ratio per release
- [ ] Aggregate statistics (mean, median, sample SD) are correct, including edge cases (N=0, N=1)
- [ ] `--repo owner/name` flag works for running outside the target repo

### Non-Functional Requirements

- [ ] All metric calculations have table-driven unit tests
- [ ] GitHub API layer is mockable via narrow role-specific interfaces
- [ ] No secrets or tokens hardcoded; no tokens or auth headers in error messages
- [ ] Rate limiting handled with retry and backoff (check headers proactively)
- [ ] Clear, actionable error messages for common failures (missing scope, not a git repo, no tags)
- [ ] Structured JSON errors when `--format json` is set (parseable by agents)
- [ ] Exit codes follow documented table (0=success, 1=general, 2=config, 3=auth, 4=not found)
- [ ] `context.Context` propagated through all API and git operations (supports Ctrl+C cancellation)
- [ ] GraphQL queries use variables only — no string interpolation of user input
- [ ] `--post` is idempotent (updates existing comment instead of creating duplicates)
- [ ] `--dry-run` flag available for `--post` operations

### Quality Gates

- [ ] `task test` passes with no failures
- [ ] `task quality` passes (lint, vet, staticcheck)
- [ ] Test coverage on `internal/metrics/` and `internal/linking/` packages
- [ ] Integration tests run against a locally built `gh-velocity` binary (via `task build`), not `go run`. The Taskfile should produce the binary and tests invoke it directly as a compiled command.

## Resolved Spec Questions

Decisions documented from todo #018. These resolve ambiguities identified during spec flow analysis.

### Q1: Cycle time with zero commits

**Decision:** Output `N/A`, exit 0 with a stderr note explaining no referencing commits were found. This aligns with the existing empty/degenerate result handling: `N/A` in pretty/markdown, `null` in JSON, warnings on stderr.

### Q2: First release with huge commit history

**Decision:** Warn on stderr if >500 commits are included in the diff (first release diffs from initial commit). Document the `--since <sha>` flag to let users limit scope to a reasonable range. The command still completes — the warning is informational, not blocking.

### Q3: Tag format acceptance

**Decision:** Exact match only. If `gh velocity release v1.0.0` is run but the actual tag is `1.0.0` (no `v` prefix), the command exits with error code 4 (not found) and a hint: `tag "v1.0.0" not found. Did you mean "1.0.0"?`. No automatic prefix stripping or fuzzy matching.

### Q4: Auto workflow detection

**Decision:** Removed. The `auto` workflow detection mode is not implemented. The default workflow is `pr`; users who work without PRs set `workflow: local` in `.gh-velocity.yml`. This was already reflected in the Triage Decisions section above.

### Q5: Idempotent marker for range-based commands

**Decision:** Use `<!-- gh-velocity:throughput:{from}:{to}:{group-by} -->` as the HTML comment marker for `throughput`. The `project` command follows the same pattern: `<!-- gh-velocity:project:{from}:{to} -->`. This extends the existing idempotent posting mechanism (documented in the Posting Research Insights section) to commands that operate on date ranges rather than single entities.

### Q6: Discussion creation vs update for `release --post`

**Decision:** Create a new discussion per release. Each release gets its own discussion (one discussion = one release). This differs from the idempotent update pattern used for issue/PR comments — releases are point-in-time events, not evolving metrics, so a new discussion per release is the natural model.

### Q7: Low label coverage warning

**Decision:** Yes, warn on stderr when >50% of issues in a release have no bug/feature labels. The warning helps users improve their labeling hygiene so release composition metrics become more meaningful over time. The command still completes successfully (exit 0) — the warning is advisory.

## Dependencies & Prerequisites

- Go 1.24+ (available via asdf)
- `gh` CLI installed and authenticated (`gh auth login`)
- For Projects v2 features: `project` scope (`gh auth refresh --scopes project`)
- For `--post` to discussions: `discussions.category_id` in config

## Risk Analysis & Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| Projects v2 GraphQL complexity | High — inline fragments, pagination, scope requirements | Phase 3 (defer until foundation solid). Use string-based `Do()` queries. |
| Rate limiting on large releases | Medium — 100+ issues = 100+ API calls | Batch where possible. Concurrent requests with bounded goroutine pool. Retry with backoff. |
| Semver assumption for quality metrics | Medium — not all repos use semver | Graceful degradation: quality metrics unavailable with warning, velocity metrics still work. |
| go-gh API surface changes | Low — v2 is stable | Pin go-gh version, update deliberately. |

## References & Research

### Internal References
- Brainstorm: `docs/brainstorms/2026-03-09-gh-velocity-brainstorm.md`
- go-calcmark metrics scripts: `/Users/bitsbyme/projects/go-calcmark/.claude/skills/github-project/scripts/`
- go-calcmark project structure: `/Users/bitsbyme/projects/go-calcmark/cmd/calcmark/`
- go-calcmark AGENTS.md: `/Users/bitsbyme/projects/go-calcmark/AGENTS.md`
- go-calcmark Taskfile: `/Users/bitsbyme/projects/go-calcmark/Taskfile.yml`
- go-calcmark golangci config: `/Users/bitsbyme/projects/go-calcmark/.golangci.yml`

### External References
- [go-gh v2 documentation](https://pkg.go.dev/github.com/cli/go-gh/v2)
- [Creating GitHub CLI Extensions](https://docs.github.com/en/github-cli/github-cli/creating-github-cli-extensions)
- [GitHub Projects v2 API](https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/using-the-api-to-manage-projects)
- [DORA Metrics](https://dora.dev/guides/dora-metrics/)
- [Compound Engineering blog post](https://bitsby.me/2026/03/compound-engineering/)
- [CalcMark Release Velocity Discussion #42](https://github.com/CalcMark/go-calcmark/discussions/42)

### Institutional Learnings (from go-calcmark docs/solutions/)
- **Test isolation**: Use `TestMain` with isolated `HOME` for packages that read user config or XDG paths. Never call config in `init()`.
- **Go closure capture**: Closures capture value-type locals, not the returned copy. Use pointer indirection for mutable state in command handlers.
- **Map iteration order**: Non-deterministic. Sort keys before iterating when output order matters (test golden files, formatted output).
- **Cobra help**: Don't override Cobra's built-in help generation. Use command groups instead.
- **File organization**: Split by logical concern. One file per subcommand in `cmd/`, separate files per metric in `internal/metrics/`.
- **Build pipeline**: Sequential `cmds` in Taskfile, not parallel `deps`. Prevent `clean` from racing with `test`/`build`.
