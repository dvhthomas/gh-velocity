---
title: "Scope & Lifecycle Filtering"
date: 2026-03-11
status: completed
type: brainstorm
---

# Scope & Lifecycle Filtering

## What We're Building

Scope and lifecycle are two foundational architectural concepts for gh-velocity:

**Scope** (user-controlled): "Which issues/PRs am I analyzing?" — repo, labels, project board, assignee, title keywords, issue types. This is the user's lens on their work. Configured in `.gh-velocity.yml` and/or narrowed further via `--scope` flag.

**Lifecycle** (command-controlled): "What workflow stage?" — each command appends the right lifecycle qualifiers internally (e.g., `is:closed closed:start..end` for lead time). Never user-configured — the command knows what stage it needs.

**Strategy** (metric-specific): "How do we measure this metric?" — determines which type qualifier (`is:issue` vs `is:pr`) and which lifecycle stages apply. Currently only cycle time needs this (`issue`, `pr`, or `project-board`).

These aren't just a "filtering feature" — they are the concepts that govern every data-fetching path in the tool. Every command assembles a query from:

```
scope:     config scope + flag scope (AND semantics)
+ lifecycle: command-specific qualifiers (includes is:issue or is:pr from strategy)
= full query sent to GitHub search API
```

Note: `repo:` is NOT automatically injected. It's part of the user's scope. Most users will include it, but omitting it enables org-wide metrics across repos.

Both scope and lifecycle use **GitHub's native search query syntax**. The key insight: they compound as a single search query. Users can build and test their scope at github.com/issues, paste it into config, and know that bolting on lifecycle qualifiers will also work.

Zero client-side filtering. The only client-side work is pagination and metric computation.

## Why This Approach

### GitHub search syntax as the filter language

Instead of inventing a custom filter DSL, we use GitHub's own search query syntax. This means:

- No custom parser to build or maintain
- Users already know the syntax (or can learn it from GitHub docs)
- Agents can reason about the query string directly
- Every qualifier GitHub adds in the future works automatically
- Users can validate their queries in the GitHub UI before putting them in config

The instruction to users is simple: "Build your query at github.com/issues. If it works there, it works here."

### API-validated qualifiers

Tested via REST search API (`/search/issues`) on 2026-03-11:

| Qualifier | Works via API | Notes |
|-----------|:---:|-------|
| `repo:owner/repo` | Yes | Scopes to a single repo |
| `label:"bug"` | Yes | AND with multiple, OR with comma |
| `type:Bug` | Yes | GitHub Issue Types (newer feature) |
| `project:owner/repo/N` | Yes | Projects v2 boards |
| `assignee:user` | Yes | `@me` supported |
| `author:user` | Yes | |
| `in:title "keyword"` | Yes | Substring match, not regex |
| `reason:completed` | Yes | Closure reason |
| `OR` (no parens) | Yes | `in:title "auth" OR in:title "login"` |
| `OR` with parens | No | Returns 0 results — not supported via API |
| `linked:pr` | Yes | Issues linked to PRs |
| `involves:user` | Yes | |

### Project status filtering via GraphQL

Tested via GraphQL `projectV2.items(query:)` on 2026-03-11:

| Query | Works | Notes |
|-------|:---:|-------|
| `status:Done` | Yes | Filters by status field value |
| `status:"In Progress"` | Yes | Quoted multi-word values |
| `-status:Done` | Yes | Negation supported |
| `label:bug` | Yes | Label filtering within project |

This means project board status filtering is **server-side** — no post-fetch needed. The `items(query:)` argument accepts a search string similar to the REST search API.

### Scope vs lifecycle separation

Commands know their lifecycle requirements. Users shouldn't need to think about `is:closed` or date ranges — those are implementation details of each metric. Users only think about "show me bugs" or "show me my team's work on project board 3."

## Key Decisions

### 1. Scope and lifecycle are architectural concepts, not a feature

Every data-fetching path respects scope — whether the command uses search (lead-time, cycle-time, throughput) or strategy-based discovery (release). For strategy-based commands, scope pre-filters the PR search that strategies already perform.

### 2. Config structure

```yaml
scope:
  query: 'repo:myorg/myrepo label:"bug"'

project:
  url: "https://github.com/users/dvhthomas/projects/1"
  status_field: "Status"

lifecycle:
  backlog:
    query: 'is:open'
    project_status: ["Backlog", "Triage", "New"]
  in-progress:
    query: 'is:open'
    project_status: ["In Progress", "Doing"]
  in-review:
    query: 'is:open'
    project_status: ["In Review"]
  done:
    query: 'is:closed'
    project_status: ["Done", "Shipped"]
  released:
    # No query — determined by tag-based discovery

cycle_time:
  strategy: pr  # or "issue" or "project-board"
```

Key design choices:
- **`scope.query`** is type-agnostic — no `is:issue` or `is:pr`. The command/strategy injects that.
- **`project.url`** uses the GitHub URL, not the internal `PVT_` ID. We resolve the internal ID at runtime.
- **`project.status_field`** uses the visible field name from the UI (e.g., "Status"), not the internal `PVTSSF_` ID.
- **`lifecycle` stages** have both a `query` (for REST search) and `project_status` (for GraphQL project filtering). Repos without project boards only use `query`. Repos with project boards use both.
- **Strategy** stays under `cycle_time.strategy` — only cycle time needs it. Lead time is always created → closed. Throughput just counts.
- **Clean break** from current config format — `project.id` and `project.status_field_id` replaced by `project.url` and `project.status_field`. No backward compatibility. Tool is pre-1.0.

### 3. `--scope` flag adds to config scope (AND semantics)

```bash
gh velocity flow lead-time --since 30d --scope 'assignee:@me'
```

The flag narrows further — it does not replace the config scope. This prevents accidentally losing your project filter when adding an ad-hoc constraint.

### 4. `--verbose` prints the full assembled query with clickable URL

```
[strategy]  pr
[scope]     repo:myorg/myrepo label:"bug"
[lifecycle] is:pr is:merged merged:2026-02-09..2026-03-11
[query]     repo:myorg/myrepo label:"bug" is:pr is:merged merged:2026-02-09..2026-03-11
[url]       https://github.com/issues?q=repo%3Amyorg%2Fmyrepo+label%3A%22bug%22+is%3Apr+is%3Amerged+merged%3A2026-02-09..2026-03-11
[result]    3 pages, 247 PRs
```

For project-board strategy, outputs a single project view URL with the filter embedded:

```
[strategy]  project-board
[scope]     repo:myorg/myrepo label:"bug"
[lifecycle] is:issue is:open, status:"In Progress"
[query]     repo:myorg/myrepo label:"bug" is:issue is:open
[project]   https://github.com/users/dvhthomas/projects/1/views/1?filterQuery=status%3A%22In+Progress%22+label%3A%22bug%22
[result]    2 pages, 42 issues
```

GitHub project views support a `filterQuery` URL parameter with the same syntax as the project filter bar (`status:"In Progress" label:bug`). This means the verbose output is a single clickable URL that shows the user the exact filtered project view. View number defaults to 1 (the default view) or can be configured.

Users can paste the URL into a browser to see exactly the same results.

### 5. No client-side filtering

All filtering is API-side:
- REST search API for scope + lifecycle query qualifiers
- GraphQL projectV2 `items(query:)` for project status filtering
- Client only handles pagination and metric computation

### 6. `in:title` instead of regex for title matching

GitHub search doesn't support regex in queries. `in:title "keyword"` does substring matching. For classification (categories), regex is still available via the existing `title:/regex/` matcher. For scoping, substring via `in:title` is sufficient.

### 7. No `OR` with parentheses

The API doesn't support grouped OR. Bare `OR` works: `in:title "auth" OR in:title "login"`. Document this limitation.

### 8. Single-item commands ignore scope

`flow lead-time #42` fetches a specific issue — scope doesn't apply. Self-evident, no special handling needed.

### 9. Preflight validates scope queries, doesn't generate them

`config preflight` does not suggest scope queries — users build those in the GitHub UI. Preflight validates configured scope queries by running them against the API and reporting result counts. It prints the full query for each lifecycle stage so users can verify:

```
Scope validation (issues):
  scope query: repo:cli/cli is:issue label:"bug" project:cli/cli/1
    → 247 issues

  + lead-time: repo:cli/cli is:issue label:"bug" project:cli/cli/1 is:closed closed:2026-02-09..2026-03-11
    → 38 issues

  + cycle-time: repo:cli/cli is:issue label:"bug" project:cli/cli/1 is:closed closed:2026-02-09..2026-03-11
    → 38 issues

  + wip: repo:cli/cli is:issue label:"bug" project:cli/cli/1 is:open
    → 12 issues

Scope validation (PRs):
  scope query: repo:cli/cli is:pr linked:issue
    → 183 PRs

  + cycle-time: repo:cli/cli is:pr linked:issue is:merged merged:2026-02-09..2026-03-11
    → 27 PRs
```

Users can copy any query into github.com/issues to verify results.

### 10. Strategy determines type qualifier injection

The `cycle_time.strategy` setting controls which type qualifier gets injected:
- `issue` strategy → inserts `is:issue`, lifecycle uses `is:closed closed:start..end`
- `pr` strategy → inserts `is:pr`, lifecycle uses `is:merged merged:start..end`
- `project-board` strategy → inserts `is:issue`, uses both REST query and GraphQL project status filtering

Other metrics don't need strategy configuration:
- Lead time: always `is:issue is:closed` (created → closed)
- Throughput: counts both issues (`is:closed`) and PRs (`is:merged`)
- WIP: always `is:issue is:open` + project status
- Release: tag-based discovery with scope pre-filtering

## Lifecycle Stages and Their Queries

| Stage | query | project_status (typical) | Used by |
|-------|-------|-------------------------|---------|
| `backlog` | `is:open` | Backlog, Triage, New | WIP (excluded) |
| `in-progress` | `is:open` | In Progress, Doing | WIP, cycle-time (start) |
| `in-review` | `is:open` | In Review | WIP, cycle-time (part of) |
| `done` | `is:closed` | Done, Shipped | Lead-time (end), cycle-time (end), throughput |
| `released` | — | — | Release metrics (tag-based) |

## Pagination

GitHub search returns max 1000 results per query. With scope filters narrowing results, most queries will be well under this limit. For large result sets:

- Page through all results automatically (transparent to user)
- Warn if 1000-result cap is hit (suggest narrowing scope)
- `--verbose` shows page count

## Resolved Questions

1. **Should `quality release` support scope filtering?** Yes. Scope is architectural — it applies everywhere. For release, scope pre-filters the PR search that strategies already perform.

2. **How does scope interact with single-item commands?** It doesn't. Single-item fetches are already maximally specific. Self-evident.

3. **Config scope validation?** Preflight validates scope queries against the API and reports result counts per lifecycle stage, printing the full query each time. Preflight does not generate scope queries.

4. **Can project status be filtered server-side?** Yes. GraphQL `projectV2.items(query: "status:Done")` supports server-side filtering including negation (`-status:Done`). No post-fetch filtering needed.

5. **Breaking change for config format?** Clean break. `project.url` replaces `project.id`, `project.status_field` replaces `project.status_field_id`. Tool is pre-1.0.

## References

- GitHub search syntax: https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests
- GitHub issue filtering UI: https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/filtering-and-searching-issues-and-pull-requests
- GitHub Projects v2 API: https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/using-the-api-to-manage-projects
- Existing search implementation: `internal/github/search.go`
- Existing classify/matcher system: `internal/classify/classify.go` (used for categories, not scope filtering)
