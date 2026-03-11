---
title: "feat: Scope & lifecycle filtering"
type: feat
status: active
date: 2026-03-11
---

# feat: Scope & Lifecycle Filtering

## Overview

Replace the hardcoded query construction throughout gh-velocity with two architectural concepts: **scope** (user-controlled filter for which issues/PRs to analyze) and **lifecycle** (command-controlled qualifiers for workflow stages). Both use GitHub's native search query syntax — no custom DSL. All filtering is API-side; the client only paginates and computes metrics.

This is a foundational refactor that touches config, every search API path, and every command's data-fetching logic.

## Problem Statement

Today, every command builds its own search query with hardcoded `repo:owner/repo is:issue is:closed closed:start..end` strings. There's no way for users to:

- Filter by label, assignee, project, or any other GitHub qualifier
- Narrow ad-hoc queries via CLI flag
- Use the same tool for org-wide metrics (no `repo:` auto-injection)
- Verify the exact query being sent to GitHub

Commands also hardcode lifecycle semantics (what "closed" or "merged" means per metric) and type qualifiers (`is:issue` vs `is:pr`), making the strategy pattern incomplete.

## Proposed Solution

### Architecture

```
scope:     config scope.query + --scope flag (AND semantics)
+ lifecycle: command-specific qualifiers (includes is:issue/is:pr from strategy)
= full query sent to GitHub search API
```

Every data-fetching path assembles a query from these two layers. `--verbose` prints the full query and a clickable GitHub URL for verification.

### Phase 1: New config format and `internal/scope` package

**Goal**: Define the config shape and a `Query` type that assembles scope + lifecycle.

#### 1a. Config changes (`internal/config/config.go`)

Replace:
- `ProjectConfig.ID` (PVT_*) → `ProjectConfig.URL` (GitHub URL)
- `ProjectConfig.StatusFieldID` (PVTSSF_*) → `ProjectConfig.StatusField` (visible name, e.g. "Status")
- `StatusConfig` (backlog/ready/in_progress/in_review/done + backlog_labels + active_labels) → `LifecycleConfig` with named stages
- Remove `FieldsConfig` (start_date/target_date — unused)

New config structs:

```go
type ScopeConfig struct {
    Query string `yaml:"query" json:"query"` // GitHub search query fragment
}

type ProjectConfig struct {
    URL         string `yaml:"url" json:"url"`                   // e.g. "https://github.com/users/dvhthomas/projects/1"
    StatusField string `yaml:"status_field" json:"status_field"` // visible name, e.g. "Status"
}

type LifecycleStage struct {
    Query         string   `yaml:"query" json:"query"`                   // REST search qualifiers
    ProjectStatus []string `yaml:"project_status" json:"project_status"` // GraphQL project status values
}

type LifecycleConfig struct {
    Backlog    LifecycleStage `yaml:"backlog" json:"backlog"`
    InProgress LifecycleStage `yaml:"in-progress" json:"in-progress"`
    InReview   LifecycleStage `yaml:"in-review" json:"in-review"`
    Done       LifecycleStage `yaml:"done" json:"done"`
    Released   LifecycleStage `yaml:"released" json:"released"`
}
```

Updated `Config` struct:

```go
type Config struct {
    Workflow    string            `yaml:"workflow" json:"workflow"`
    Scope       ScopeConfig       `yaml:"scope" json:"scope"`
    Project     ProjectConfig     `yaml:"project" json:"project"`
    Lifecycle   LifecycleConfig   `yaml:"lifecycle" json:"lifecycle"`
    Quality     QualityConfig     `yaml:"quality" json:"quality"`
    Discussions DiscussionsConfig `yaml:"discussions" json:"discussions"`
    CommitRef   CommitRefConfig   `yaml:"commit_ref" json:"commit_ref"`
    CycleTime   CycleTimeConfig   `yaml:"cycle_time" json:"cycle_time"`
}
```

Remove from Config: `Statuses`, `Fields`.

Remove from `knownTopLevelKeys`: `"statuses"`, `"fields"`. Add: `"scope"`, `"lifecycle"`.

Remove validation: `projectIDPattern` (PVT_*), `statusFieldIDPattern` (PVTSSF_*). Add: `project.url` must be a valid GitHub project URL when set. `project.status_field` required when any lifecycle stage uses `project_status`.

**How lifecycle works**: Commands know *which stage* to use (e.g., lead-time uses `done`, WIP uses the negation of backlog/done/released). The user defines *what each stage means* — the query qualifiers and/or project board statuses. Commands never hardcode lifecycle qualifiers like `is:closed`; they always read from config.

Two execution paths per stage:
- **`query` only** (no `project_status`): run through REST Search API. The stage's query is appended to scope.
- **`project_status` set** (with or without `query`): run through GraphQL project items `items(query:)`. If `query` is also provided, both are used together — the REST query filters items, and `project_status` filters within the project board.

Default lifecycle (when not configured) — sensible GitHub-isms that work for any repo:
```go
func defaultLifecycle() LifecycleConfig {
    return LifecycleConfig{
        Backlog:    LifecycleStage{Query: "is:open"},
        InProgress: LifecycleStage{Query: "is:open"},
        InReview:   LifecycleStage{Query: "is:open"},
        Done:       LifecycleStage{Query: "is:closed"},
        // Released: no default — tag-based discovery, no query needed
    }
}
```

With just these defaults and auto-injected `repo:`, a zero-config user gets:
- **lead-time**: `repo:owner/repo is:issue is:closed closed:start..end` — works
- **cycle-time**: `repo:owner/repo is:issue is:closed closed:start..end` (issue strategy default) — works
- **throughput**: issues `repo:owner/repo is:issue is:closed closed:start..end` + PRs `repo:owner/repo is:pr is:merged merged:start..end` — works
- **WIP**: `repo:owner/repo is:issue is:open` (minus backlog/done negation, which are no-ops without project_status) — works
- **release**: tag-based discovery, scope only — works

**Override semantics**: If the user configures any lifecycle stage or scope, it completely replaces the internal default for that field. No merging, no layering. A user who sets `done.query: "is:closed reason:completed"` gets exactly that — the default `is:closed` is gone. Same for scope: if `scope.query` is set, no `repo:` auto-injection.

- [x] Update `Config` struct — remove `Statuses`, `Fields`, add `Scope`, update `Project`, add `Lifecycle`
- [x] Update `defaults()` — new default lifecycle
- [x] Update `knownTopLevelKeys` — remove `"statuses"`, `"fields"`, add `"scope"`, `"lifecycle"`
- [x] Update `validate()` — new project URL validation, lifecycle stage validation, remove PVT_*/PVTSSF_* regex
- [x] Update config tests — all table-driven tests need new config format

**Tests**: Table-driven validation for new config fields. Invalid project URL. Missing `status_field` when `project_status` used. Valid minimal config. Full config.

#### 1b. New `internal/scope` package

This package owns query assembly. It has zero API dependency — pure string manipulation.

```go
// Package scope assembles GitHub search queries from config and flags.
package scope

// Query holds the components of a search query.
type Query struct {
    Scope     string // user scope from config + flag
    Lifecycle string // command lifecycle qualifiers
    Type      string // "is:issue" or "is:pr" (from strategy)
}

// Build assembles the full search query string.
func (q Query) Build() string {
    // Joins non-empty parts with spaces
}

// URL returns a clickable GitHub search URL.
func (q Query) URL() string {
    // https://github.com/issues?q=url.QueryEscape(q.Build())
}

// String returns the full query (alias for Build).
func (q Query) String() string

// Verbose returns a multi-line diagnostic string.
// [scope]     repo:myorg/myrepo label:"bug"
// [lifecycle] is:closed closed:2026-02-09..2026-03-11
// [type]      is:issue
// [query]     repo:myorg/myrepo label:"bug" is:issue is:closed closed:2026-02-09..2026-03-11
// [url]       https://github.com/issues?q=...
func (q Query) Verbose() string
```

Helper to merge config scope with flag scope:

```go
// MergeScope combines config scope and flag scope with AND semantics.
// Both are GitHub search query fragments; they're joined with a space.
func MergeScope(configScope, flagScope string) string
```

- [x] Create `internal/scope/scope.go` — `Query` type, `Build()`, `URL()`, `Verbose()`
- [x] Create `internal/scope/scope_test.go` — table-driven tests for query assembly, URL encoding, verbose output

**Tests**: Empty scope. Scope + lifecycle. Scope + lifecycle + type. URL encoding. Verbose format. MergeScope with both, one, neither.

#### 1c. Project URL resolution

When `project.url` is set, resolve the internal project number and node ID at runtime via GraphQL.

```go
// internal/github/project.go (new or extend projectitems.go)

// ResolveProjectURL parses a GitHub project URL and returns the project number.
// Accepts: https://github.com/users/{user}/projects/{N}
//          https://github.com/orgs/{org}/projects/{N}
func ParseProjectURL(rawURL string) (owner string, number int, isOrg bool, err error)

// ResolveProject fetches the project node ID and status field ID from a URL.
func (c *Client) ResolveProject(ctx context.Context, projectURL, statusFieldName string) (projectID, statusFieldID string, err error)
```

- [x] Add `ParseProjectURL()` in `internal/github/project.go`
- [x] Add `ResolveProject()` GraphQL query to get node ID + status field ID by visible name
- [x] Tests for URL parsing (user projects, org projects, invalid URLs)

### Phase 2: Refactor search API to accept assembled queries

**Goal**: Replace all hardcoded query construction in `internal/github/search.go` and `pullrequests.go` with a single generic search function that accepts a pre-built query string.

#### 2a. Generic search function

```go
// SearchIssues executes a GitHub search API query and returns issues.
// The query must be a complete, pre-assembled search string.
func (c *Client) SearchIssues(ctx context.Context, query string) ([]model.Issue, error)

// SearchPRs executes a GitHub search API query and returns PRs.
func (c *Client) SearchPRs(ctx context.Context, query string) ([]model.PR, error)
```

Both share the same pagination logic (currently duplicated across `SearchClosedIssues`, `SearchOpenIssuesWithLabels`, `SearchMergedPRs`).

```go
// searchPaginated runs a paginated search and returns raw items.
func (c *Client) searchPaginated(ctx context.Context, query string) ([]searchIssueResponse, error) {
    // Shared pagination: 100 per page, max 10 pages, warn on cap
}
```

- [x] Create `searchPaginated()` — extract shared pagination logic
- [x] Create `SearchIssues()` — uses `searchPaginated()`, converts to `model.Issue`
- [x] Create `SearchPRs()` — uses `searchPaginated()`, converts to `model.PR`
- [x] Deprecate `SearchClosedIssues`, `SearchOpenIssuesWithLabels`, `SearchMergedPRs` (keep as thin wrappers initially, remove in follow-up)
- [ ] Add `--verbose` page count reporting to search functions
- [x] `Client` struct: `SearchIssues`/`SearchPRs` do NOT use `c.owner`/`c.repo` — the query is pre-assembled. `Client` still needs owner/repo for non-search API calls (GetIssue, GetPR, tags, releases, etc.), so `NewClient(owner, repo)` signature stays.

**Tests**: Mock-based tests for pagination. Query passthrough (no modification).

#### 2b. Project items with query filter

Update `ListProjectItems` to accept a `query` string parameter for server-side filtering:

```go
// ListProjectItems fetches items from a project board, optionally filtered by query.
// The query uses the same syntax as the project filter bar: "status:Done label:bug"
func (c *Client) ListProjectItems(ctx context.Context, projectID, statusFieldID, query string) ([]model.ProjectItem, error)
```

- [ ] Update `ListProjectItems` to pass `query` in the GraphQL `items(query:)` argument
- [ ] Update callers (currently `cmd/wip.go`)

### Phase 3: Wire scope + lifecycle into commands

**Goal**: Every command assembles a `scope.Query` and uses the generic search functions.

#### 3a. Add `--scope` flag to root command (`cmd/root.go`)

```go
// In NewRootCmd:
var scopeFlag string
root.PersistentFlags().StringVar(&scopeFlag, "scope", "", "Additional scope filter (GitHub search syntax, AND with config)")

// In Deps:
type Deps struct {
    // ...existing...
    Scope string // merged config + flag scope
}

// In PersistentPreRunE:
deps.Scope = scope.MergeScope(cfg.Scope.Query, scopeFlag)
```

- [x] Add `--scope` flag to root command
- [x] Add `Scope` field to `Deps`
- [x] Merge config + flag scope in `PersistentPreRunE`
- [x] Update `--debug` output to show scope

#### 3b. Lead time (`cmd/leadtime.go`)

Bulk mode currently calls `client.SearchClosedIssues(ctx, since, until)`.

Replace with lifecycle-config-driven query assembly:

```go
// Lead time uses the "done" lifecycle stage — user configures what "done" means.
doneStage := deps.Config.Lifecycle.Done
q := scope.Query{
    Scope:     deps.Scope,
    Type:      "is:issue",
    Lifecycle: doneStage.Query + " " + fmt.Sprintf("closed:%s..%s", sinceStr, untilStr),
}
if deps.Debug {
    fmt.Fprint(os.Stderr, q.Verbose())
}
issues, err := client.SearchIssues(ctx, q.Build())
// If doneStage.ProjectStatus is set, also filter via project board
```

The command knows it needs the "done" stage and appends the date range. The user's lifecycle config provides the qualifiers (e.g., `is:closed` or `is:closed reason:completed`).

- [x] Refactor `runLeadTimeBulk` to use `scope.Query` + `SearchIssues`
- [x] Add verbose query output when `--debug`

#### 3c. Cycle time (`cmd/cycletime.go`)

Bulk mode currently calls `client.SearchClosedIssues` for issue strategy and `client.SearchMergedPRs` for PR strategy.

Replace with strategy-aware query assembly:

```go
var q scope.Query
switch deps.Config.CycleTime.Strategy {
case "pr":
    q = scope.Query{
        Scope:     deps.Scope,
        Type:      "is:pr",
        Lifecycle: fmt.Sprintf("is:merged merged:%s..%s", sinceStr, untilStr),
    }
    prs, err := client.SearchPRs(ctx, q.Build())
case "issue":
    q = scope.Query{
        Scope:     deps.Scope,
        Type:      "is:issue",
        Lifecycle: fmt.Sprintf("is:closed closed:%s..%s", sinceStr, untilStr),
    }
    issues, err := client.SearchIssues(ctx, q.Build())
case "project-board":
    // REST search for open issues, then GraphQL project filter
    q = scope.Query{
        Scope:     deps.Scope,
        Type:      "is:issue",
        Lifecycle: deps.Config.Lifecycle.Done.Query + " " + fmt.Sprintf("closed:%s..%s", sinceStr, untilStr),
    }
    issues, err := client.SearchIssues(ctx, q.Build())
    // Then filter via project status if configured
}
```

- [x] Refactor `runCycleTimeBulk` — strategy-aware scope.Query assembly
- [ ] Handle project-board strategy with GraphQL project status filter
- [x] Add verbose output

#### 3d. Throughput (`cmd/throughput.go`)

Currently makes two calls: `SearchClosedIssues` + `SearchMergedPRs`.

Replace with two scope-aware queries:

```go
issueQ := scope.Query{
    Scope:     deps.Scope,
    Type:      "is:issue",
    Lifecycle: fmt.Sprintf("is:closed closed:%s..%s", sinceStr, untilStr),
}
prQ := scope.Query{
    Scope:     deps.Scope,
    Type:      "is:pr",
    Lifecycle: fmt.Sprintf("is:merged merged:%s..%s", sinceStr, untilStr),
}
```

- [x] Refactor throughput to use `scope.Query` for both issue and PR queries
- [x] Add verbose output

#### 3e. WIP (`cmd/wip.go`)

Currently uses `SearchOpenIssuesWithLabels` + `ListProjectItems`.

**WIP is a negation**: WIP means "everything in scope that is NOT in backlog, done, or released." Today that equates to `in-progress` + `in-review`, but if new stages are added later (e.g., `on-hold`), the negation still works — anything not explicitly excluded is WIP.

Implementation approach for REST search path:
```go
// WIP = scope + is:open + NOT done/backlog/released
// GitHub search supports negation: -label:"backlog"
// For project-board: use GraphQL items(query:) with negated statuses
//   e.g., "-status:Done -status:Backlog"
```

For project-board path, build a negation query from lifecycle stages:
```go
// Collect statuses to exclude from lifecycle config
var excludeStatuses []string
excludeStatuses = append(excludeStatuses, cfg.Lifecycle.Backlog.ProjectStatus...)
excludeStatuses = append(excludeStatuses, cfg.Lifecycle.Done.ProjectStatus...)
// Build: -status:"Done" -status:"Shipped" -status:"Backlog"
```

For REST-only path (no project board), WIP builds a query from scope + `is:open`. Done/released items are already excluded by `is:open`. For backlog exclusion without a project board, the user can put negation qualifiers in their `lifecycle.backlog.query` (e.g., `is:open -label:"backlog" -label:"icebox"`). WIP's query becomes: scope + `is:open` + negate(backlog.query). If backlog has no query configured, WIP is simply scope + `is:open`.

- [ ] Implement WIP as negation of backlog/done/released lifecycle stages
- [ ] Project-board path: negate project statuses via GraphQL `items(query:)`
- [ ] REST-only path: `is:open` with scope (backlog label exclusion if configured)
- [ ] Ensure future lifecycle stages (e.g., on-hold) are automatically included in WIP

#### 3f. Quality release (`cmd/release.go`)

Release uses the strategy pattern (`prlink`, `commitref`, `changelog`) to discover items. `prlink.Discover()` internally calls `SearchMergedPRs` — scope must flow into that PR search so only PRs matching the user's scope are discovered.

Add scope to `DiscoverInput`:

```go
type DiscoverInput struct {
    // ...existing...
    Scope string // user scope query fragment (pre-filters PR search)
}
```

Update `prlink.go` to prepend scope to its merged PR search query.

**Flag collision**: The existing `--scope` flag on `quality release` (line 26 of release.go) shows a *diagnostic view* of what each strategy found. This collides with the new root-level `--scope` filter flag. Rename the diagnostic flag to `--discover` or `--strategy-view` to avoid confusion.

After discovery, the classifier (`classify.NewClassifier`) and quality metrics still work as before — scope only affects which items are *discovered*, not how they're classified.

- [x] Add `Scope` to `strategy.DiscoverInput`
- [x] Update `prlink.Discover()` to include scope in PR search
- [x] Rename `--scope` diagnostic flag to `--discover` (breaking but pre-1.0)
- [ ] Add verbose output in strategy runner
- [x] Verify classifier and quality metrics still work with scope-filtered items

#### 3g. Report (`cmd/report.go`)

Report is a composite — it calls the other commands' logic. Scope flows through `Deps.Scope` automatically.

- [x] Verify report correctly inherits scope from Deps

### Phase 4: Verbose output

**Goal**: `--debug` shows the assembled query, lifecycle, strategy, and a clickable URL.

The `scope.Query.Verbose()` method produces the diagnostic output. Each command prints it to stderr before executing the search.

Format:
```
[strategy]  pr
[scope]     repo:myorg/myrepo label:"bug"
[lifecycle] is:pr is:merged merged:2026-02-09..2026-03-11
[query]     repo:myorg/myrepo label:"bug" is:pr is:merged merged:2026-02-09..2026-03-11
[url]       https://github.com/issues?q=...
```

For project-board strategy, also show the project filter URL:
```
[project]   https://github.com/users/dvhthomas/projects/1/views/1?filterQuery=status%3A%22In+Progress%22
```

- [ ] Add project URL builder to `scope.Query` (for project-board verbose output)
- [ ] Wire verbose output into all commands via `deps.Debug`
- [ ] Show page count after search completes: `[result] 3 pages, 247 issues`

### Phase 5: Update preflight

**Goal**: Preflight generates the new config format and validates scope queries against the API.

#### 5a. Config generation

Preflight auto-detects repo characteristics and generates the new config shape:

- `scope.query`: always `repo:owner/repo` (user narrows from there)
- `project`: detect project boards, emit `url` and `status_field`
- `lifecycle`: if project board found, populate `project_status` arrays from actual board column names. If no project, emit `done.query: "is:closed"` default only.
- `quality`: existing categories/label detection (already working from harden-preflight)

- [ ] Update `renderPreflightConfig` to emit `scope:`, `project:` (URL format), `lifecycle:` (with project_status from detected board columns)
- [ ] Remove old config fields (`statuses:`, `fields:`, `project.id`, `project.status_field_id`)
- [ ] Round-trip validate generated config through `config.Parse()` (already established pattern)

#### 5b. Scope validation

After generating config, validate scope queries against the API:

- Run scope + each lifecycle stage query with `per_page=1` to check for API errors and get total counts
- Print the full composed query for each stage so users can paste into github.com/issues
- Report result counts per stage (e.g., "done: 38 issues, in-progress: 12 issues")
- Warn if any stage returns 0 results (possible misconfiguration)

- [ ] Add scope validation: run config scope + each lifecycle stage against API
- [ ] Print full query per lifecycle stage with result count
- [ ] Warn on zero-result stages
- [ ] Update preflight JSON to include `scope_validation` with per-stage counts and queries

### Phase 6: Update smoke tests and documentation

- [ ] Update `scripts/smoke-test.sh` for new config format
- [ ] Update help text in all commands to reference `--scope`
- [ ] Update `--debug` output in all commands

## Technical Considerations

### Breaking changes

This is a **clean break** from the current config format. The tool is pre-1.0, so this is acceptable.

| Old | New | Migration |
|-----|-----|-----------|
| `project.id: PVT_abc` | `project.url: https://github.com/...` | Manual edit |
| `project.status_field_id: PVTSSF_abc` | `project.status_field: "Status"` | Manual edit |
| `statuses:` block | `lifecycle:` block | Manual edit |
| `fields:` block | Removed | Delete |

`config preflight` generates the new format, so users can re-run it to migrate.

### Query assembly safety

The `scope.Query.Build()` method is pure string concatenation — no escaping or sanitization. This is intentional: GitHub search syntax is opaque to us. The user's scope is passed through verbatim to the API. If it's invalid, GitHub returns an error which we surface.

### Pagination

The existing pagination logic (100/page, max 10 pages, 1000 result cap) is preserved but centralized in `searchPaginated`. The 1000-result cap warning should include the full query so users know what to narrow.

### Project URL resolution caching

`ResolveProject()` makes a GraphQL call to resolve URL → node ID. This should be cached per command execution (not per query) since the project doesn't change during a run. Store resolved IDs on `Deps` after first resolution.

### `repo:` auto-injection

**Decision**: When `scope.query` is empty (no config or blank) and no `--scope` flag is provided, auto-inject `repo:owner/repo` into scope from `--repo` / GH_REPO / git remote. This preserves zero-config usage — `gh velocity flow lead-time --since 30d` continues to work without a config file.

Users who want org-wide metrics set `scope.query` to something that doesn't include `repo:` (e.g., `org:myorg label:"bug"`). Once scope is explicitly configured, no auto-injection occurs.

### Duplicate qualifier conflict detection

If `scope.query` contains `is:issue` and `cycle_time.strategy: pr` injects `is:pr`, the composed query becomes `is:issue is:pr` — zero results. The `scope.Query.Build()` method should detect type qualifier conflicts and return an error.

Detection: scan scope for `is:issue`, `is:pr`, `is:merged`, `is:closed`, `is:open`. If scope contains a type qualifier that conflicts with the lifecycle/strategy injection, error with a clear message: `"scope contains 'is:issue' but cycle_time.strategy 'pr' requires 'is:pr' — remove the type qualifier from scope"`.

Similarly, if scope contains date qualifiers (`closed:`, `merged:`, `created:`), warn that they may conflict with lifecycle date ranges.

### Zero results handling

When a search returns zero results, warn on stderr: `"0 results for query: <full query>. Verify your scope at <url>"`. This helps users debug scope misconfiguration without silently returning empty output.

### Project URL resolution variants

GitHub project URLs come in three forms:
- `https://github.com/users/{user}/projects/{N}` → GraphQL `user(login:) { projectV2(number:) }`
- `https://github.com/orgs/{org}/projects/{N}` → GraphQL `organization(login:) { projectV2(number:) }`
- `https://github.com/{owner}/{repo}/projects/{N}` → repo-level project (legacy, less common)

`ParseProjectURL` must handle all three. `ResolveProject` uses different GraphQL queries based on the URL form.

### Status field name resolution

`project.status_field: "Status"` is the visible name. To get the internal `PVTSSF_*` field ID, query the project's fields via GraphQL:
```graphql
projectV2(number: N) {
  field(name: "Status") { ... on ProjectV2SingleSelectField { id } }
}
```

Cache the resolved field ID alongside the project ID on `Deps`.

### GitHub Search API rate limits

Search API has a separate rate limit (30 requests/minute for authenticated users). With scope + lifecycle, each command makes fewer calls (one per metric vs one per hardcoded variant). Monitor with existing rate limit detection in `internal/github/ratelimit.go`.

## Acceptance Criteria

- [ ] New config format loads and validates correctly
- [ ] `scope.query` flows into all search API calls
- [ ] `--scope` flag narrows config scope (AND semantics)
- [ ] `--debug` prints full query, lifecycle, strategy, and clickable URL for every search
- [ ] `project.url` resolved to internal ID at runtime
- [ ] `project.status_field` resolved to field ID by visible name
- [ ] `lifecycle` stages used by each command for the correct workflow stage
- [ ] `cycle_time.strategy` controls `is:issue` vs `is:pr` injection
- [ ] `quality release` passes scope into strategy discovery (prlink PR search)
- [ ] `quality release --scope` renamed to `--discover` (no collision with root `--scope`)
- [ ] WIP is negation of backlog/done/released (future stages auto-included)
- [ ] Duplicate type qualifier in scope + strategy detected and errors
- [ ] Zero-result searches warn with full query and clickable URL
- [ ] Pagination centralized and warns at 1000-result cap with full query
- [ ] All existing tests pass (updated for new config format)
- [ ] Smoke tests pass with new config format
- [ ] `task quality` passes

## Dependencies & Risks

- **Risk**: Breaking config change. Mitigated by pre-1.0 status and `config preflight` generating new format.
- **Risk**: GitHub search API rate limits with scope validation in preflight. Mitigated by reusing existing rate limit detection.
- **Risk**: Project URL resolution adds a GraphQL call per command. Mitigated by caching on Deps.
- **Dependency**: Phase 1 (config + scope package) must complete before Phase 3 (command wiring).
- **Dependency**: Phase 2 (generic search) must complete before Phase 3.

## References

- Brainstorm: `docs/brainstorms/2026-03-11-scope-and-lifecycle-filtering-brainstorm.md`
- Current search: `internal/github/search.go:41-76` (SearchClosedIssues)
- Current PR search: `internal/github/pullrequests.go:40` (SearchMergedPRs)
- Project items: `internal/github/projectitems.go:94` (ListProjectItems)
- Strategy pattern: `internal/strategy/strategy.go:16-21` (Strategy interface)
- Config: `internal/config/config.go:37-46` (Config struct)
- Root command: `cmd/root.go:29-41` (Deps struct)
- GitHub search docs: https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests
