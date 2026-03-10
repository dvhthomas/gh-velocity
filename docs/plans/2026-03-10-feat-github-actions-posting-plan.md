---
title: "feat: GitHub Actions integration and --post flag"
type: feat
status: active
date: 2026-03-10
issue: https://github.com/dvhthomas/gh-velocity/issues/11
brainstorm: docs/brainstorms/2026-03-10-github-actions-and-posting-brainstorm.md
---

# feat: GitHub Actions Integration and --post Flag

## Overview

Enable gh-velocity to post metric output directly to GitHub (issue/PR comments for single items, Discussion posts for bulk/report commands) via the `--post` flag. Add structured CI logging for GitHub Actions, a shared stderr helper, and extend `config preflight` to check posting readiness.

This is the first write operation in the codebase — all existing GitHub API calls are reads.

## Problem Statement

Users want to automate velocity reporting in CI. Today they can compute metrics and pipe JSON, but cannot post results back to GitHub without custom scripting. The `--post` flag has been declared and hidden since the initial release, with `internal/posting/` as a planned but empty package.

## Proposed Solution

### Posting targets by command type

| Command type | Post target | Marker key | Requires |
| --- | --- | --- | --- |
| Single issue (`flow lead-time 42`) | Issue comment | `lead-time:42` | `issues:write` |
| Single PR (`flow cycle-time --pr 5`) | PR comment | `cycle-time:pr-5` | `issues:write` |
| Bulk (`flow lead-time --since 30d`) | Discussion | `lead-time:30d` | `discussions:write` + `category_id` |
| Report (`report --since 30d`) | Discussion | `report:30d` | `discussions:write` + `category_id` |
| Release (`quality release v1.0`) | Discussion | `release:v1.0` | `discussions:write` + `category_id` |

For absolute date ranges: `report:2026-01-01..2026-02-01`

### Marker format

```html
<!-- gh-velocity:lead-time:42 -->
| Metric | Value |
| --- | --- |
| Lead Time | 3d 4h 12m |
<!-- /gh-velocity -->
```

Markers constitute the **entire comment/discussion body**. On update, the full body is replaced. The closing tag is present for robustness and future extensibility.

### Flag behavior

- `--post` writes markdown to stdout AND posts to GitHub. Confirmation on stderr.
- `--post` coerces format to `markdown` unless `-f json` is explicitly set (JSON is wrapped in a code fence when posted).
- `-f pretty --post` is treated as `--post` (markdown).
- `--new-post` implies `--post`. Creates a new comment/discussion regardless of existing markers.
- Without `--post`, behavior is unchanged (read-only).

### CI integration

When `GITHUB_ACTIONS=true` (set automatically by Actions), stderr messages use workflow commands:
```
::notice::Posted to cli/cli#42 (updated)
::error::--post failed: token lacks discussions:write permission%0A%0AAdd to your workflow:%0A  permissions:%0A    discussions: write
::warning::results capped at 1000; narrow the date range
```

## Technical Approach

### Architecture

```
cmd/                          # --post flag handling in PersistentPreRunE + each command
internal/posting/             # NEW: posting logic
  poster.go                   #   Poster interface + CommentPoster + DiscussionPoster
  marker.go                   #   Marker parsing/wrapping
  marker_test.go              #   Table-driven marker tests
internal/github/              # New mutation methods on Client
  comments.go                 #   CreateComment, UpdateComment, ListComments
  discussions.go              #   CreateDiscussion, UpdateDiscussion, SearchDiscussions
internal/log/                 # NEW: structured stderr helper
  log.go                      #   Warn(), Error(), Notice(), Debug() — CI-aware
internal/model/errors.go      # New ErrPostFailed error code
internal/config/config.go     # (no changes needed — DiscussionsConfig already exists)
cmd/preflight.go              # Extend PreflightResult with PostingReadiness
```

### Implementation Phases

#### Phase 1: Foundation (stderr helper + error code + marker logic)

Build the infrastructure that everything else depends on.

- [x] **`internal/log/log.go`**: Shared stderr helper
  - `Warn(format, args...)` — `::warning::` in CI, `warning:` locally
  - `Error(format, args...)` — `::error::` in CI, plain stderr locally
  - `Notice(format, args...)` — `::notice::` in CI, plain stderr locally
  - `Debug(format, args...)` — `[debug]` prefix (same in CI and local)
  - Detection: `os.Getenv("GITHUB_ACTIONS") == "true"`
  - URL-encode newlines as `%0A` for multi-line CI messages
- [x] **`internal/log/log_test.go`**: Table-driven tests for CI vs local formatting
- [x] **`internal/model/errors.go`**: Add `ErrPostFailed = "POST_FAILED"` with exit code 1
- [x] **`internal/posting/marker.go`**: Marker utilities
  - `WrapWithMarker(command, context, content string) string` — wraps content in markers
  - `FindMarker(body, command, context string) bool` — checks if body contains marker
  - `MarkerKey(command, context string) string` — returns `gh-velocity:{command}:{context}`
- [x] **`internal/posting/marker_test.go`**: Table-driven tests
  - Happy path: wrap and find
  - Missing closing tag: still findable by opening tag
  - Multiple markers: only matches exact command+context
  - Empty body, empty content
  - Markers with special characters in context (tag names with dots, slashes)
- [x] **Migrate existing stderr calls** to use `log.Warn()`, `log.Debug()`, `log.Notice()`
  - `cmd/root.go` — debug output
  - `cmd/preflight.go` — progress messages
  - `cmd/release.go` — shallow clone warnings
  - `cmd/helpers.go` — general warnings
  - `cmd/cycletime.go` — PR search and cycle-time warnings
  - `internal/github/search.go` — result cap warnings
  - `internal/git/git.go` — malformed date warning
  - `internal/gitdata/gitdata.go` — API fallback warning
  - `internal/strategy/prlink.go` — PR count cap warning
  - `internal/config/config.go` — WarnFunc now uses log.Warn

**Tests**: `go test ./internal/log/... ./internal/posting/...`

#### Phase 2: GitHub API mutations (comments + discussions)

Add the first write operations to the GitHub client.

- [ ] **`internal/github/comments.go`**: Issue/PR comment CRUD
  - `ListComments(ctx, number int) ([]Comment, error)` — `GET /repos/{owner}/{repo}/issues/{number}/comments`
  - `CreateComment(ctx, number int, body string) error` — `POST /repos/{owner}/{repo}/issues/{number}/comments`
  - `UpdateComment(ctx, commentID int64, body string) error` — `PATCH /repos/{owner}/{repo}/issues/comments/{id}`
  - `Comment` struct: `ID int64`, `Body string`, `User string`
- [ ] **`internal/github/comments_test.go`**: Unit tests with mocked REST client
- [ ] **`internal/github/discussions.go`**: Discussion CRUD via GraphQL
  - `SearchDiscussions(ctx, categoryID string, limit int) ([]Discussion, error)` — query discussions in category, ordered by updatedAt desc
  - `CreateDiscussion(ctx, categoryID, title, body string) (string, error)` — `createDiscussion` mutation, returns discussion URL
  - `UpdateDiscussion(ctx, discussionID, body string) error` — `updateDiscussion` mutation
  - `CheckDiscussionsEnabled(ctx) (bool, error)` — `GET /repos/{owner}/{repo}` → check `has_discussions`
  - `Discussion` struct: `ID string`, `Title string`, `Body string`, `URL string`
  - Search limit: 50 most recent discussions (paginate via GraphQL cursor)
- [ ] **`internal/github/discussions_test.go`**: Unit tests with mocked GraphQL client

**Note**: GraphQL mutations use variables only (per CLAUDE.md). The `repositoryId` is fetched via `query { repository(owner:$o, name:$n) { id } }` and cached on the Client.

**Tests**: `go test ./internal/github/...`

#### Phase 3: Poster abstraction (`internal/posting/`)

Build the posting logic that commands will call.

- [ ] **`internal/posting/poster.go`**: Posting dispatch
  ```go
  // Poster posts metric output to GitHub.
  type Poster interface {
      Post(ctx context.Context, opts PostOptions) error
  }

  type PostOptions struct {
      Command   string // "lead-time", "cycle-time", "report", "release"
      Context   string // "42", "pr-5", "30d", "v1.0", "2026-01-01..2026-02-01"
      Content   string // markdown body (already formatted)
      Target    Target // IssueComment, PRComment, or Discussion
      Number    int    // issue/PR number (for comment targets)
      ForceNew  bool   // --new-post: skip search, always create
  }

  type Target int
  const (
      IssueComment Target = iota
      PRComment
      Discussion
  )
  ```
  - `CommentPoster` — implements `Poster` for issue/PR comments
    - List comments → search for marker → create or update
  - `DiscussionPoster` — implements `Poster` for Discussions
    - Search discussions → find marker → create or update
    - Requires `categoryID` from config
    - Title format: `gh-velocity {command}: {repo} ({date})`
- [ ] **`internal/posting/poster_test.go`**: Table-driven tests with mock GitHub client
  - Create new comment (no existing marker)
  - Update existing comment (marker found)
  - Force new comment (--new-post)
  - Create new discussion (no existing marker)
  - Update existing discussion (marker found)
  - Force new discussion (--new-post)
  - Missing category_id → clear error
  - Locked issue → `POST_FAILED` error with helpful message
  - Deleted category → `POST_FAILED` error

**Tests**: `go test ./internal/posting/...`

#### Phase 4: Wire --post into commands

Connect the posting infrastructure to every command.

- [ ] **`cmd/root.go`**: PersistentPreRunE changes
  - Remove the `--post` rejection guard (lines 113-117)
  - Unhide the `--post` flag (remove `MarkHidden`)
  - Add `--new-post` flag: `root.PersistentFlags().BoolVar(&newPostFlag, "new-post", false, "Force a new post (skip idempotent update)")`
  - `--new-post` implies `--post`: if `newPostFlag { postFlag = true }`
  - `--post` coerces format: if `postFlag && !cmd.Flags().Changed("format") { formatFlag = "markdown" }`
  - Store `NewPost bool` in Deps
- [ ] **Post helper in `cmd/helpers.go`** (or new `cmd/post.go`):
  ```go
  // postIfEnabled captures output via io.MultiWriter and posts after formatting.
  func postIfEnabled(cmd *cobra.Command, deps *Deps, client *gh.Client, opts posting.PostOptions) (io.Writer, func() error) {
      if !deps.Post {
          return cmd.OutOrStdout(), func() error { return nil }
      }
      var buf bytes.Buffer
      w := io.MultiWriter(cmd.OutOrStdout(), &buf)
      return w, func() error {
          opts.Content = wrapForPost(buf.String(), deps.Format)
          poster := posting.NewPoster(client, deps.Config)
          if err := poster.Post(cmd.Context(), opts); err != nil {
              return err
          }
          log.Notice("Posted to %s/%s#%d (updated)", deps.Owner, deps.Repo, opts.Number)
          return nil
      }
  }

  // wrapForPost wraps content: if JSON, adds code fence; if markdown, uses as-is.
  func wrapForPost(content string, f format.Format) string {
      if f == format.JSON {
          return "```json\n" + content + "```\n"
      }
      return content
  }
  ```
- [ ] **Update each command** to use the post helper:
  - `cmd/leadtime.go` — single issue: `IssueComment`, bulk: `Discussion`
  - `cmd/cycletime.go` — single issue: `IssueComment`, single PR: `PRComment`, bulk: `Discussion`
  - `cmd/throughput.go` — bulk only: `Discussion`
  - `cmd/report.go` — bulk only: `Discussion`
  - `cmd/release.go` — `Discussion`
  - Pattern for each command:
    ```go
    w, postFn := postIfEnabled(cmd, deps, client, posting.PostOptions{
        Command:  "lead-time",
        Context:  strconv.Itoa(issueNum),
        Target:   posting.IssueComment,
        Number:   issueNum,
        ForceNew: deps.NewPost,
    })
    // existing format switch uses w instead of cmd.OutOrStdout()
    // after format write:
    if err := postFn(); err != nil {
        return err
    }
    ```
- [ ] **Validate config for Discussion posting**: In PersistentPreRunE or in the post helper, check that `discussions.category_id` is configured when the target is Discussion. Return clear error with instructions.

**Tests**: `go test ./cmd/...` + update smoke tests

#### Phase 5: Preflight posting readiness

Extend config preflight to check posting prerequisites.

- [ ] **`cmd/preflight.go`**: Add `PostingReadiness` struct and checks
  ```go
  type PostingReadiness struct {
      DiscussionsEnabled bool  `json:"discussions_enabled"`
      HasIssuesWrite     bool  `json:"has_issues_write"`
      HasDiscussionsWrite bool `json:"has_discussions_write"`
      CategoryValid      *bool `json:"category_valid"` // nil = not configured
  }
  ```
  - Check `discussions_enabled`: `GET /repos/{owner}/{repo}` → `has_discussions` field
  - Check `has_issues_write`: attempt `GET /repos/{owner}/{repo}/issues/comments?per_page=1` — if 403, no write access (note: this only proves read; actual write scope may differ with fine-grained PATs)
  - Check `category_valid`: if `category_id` in config, attempt GraphQL query for that node — if found, valid
  - Scope checks are **best-effort** (fine-grained PATs don't expose scopes via headers)
  - Add hints for each check result
- [ ] **Update `renderPreflightConfig`** to show posting readiness in pretty output
- [ ] **Update smoke tests** to verify preflight JSON includes `posting_readiness`

**Tests**: `go test ./cmd/...` + smoke tests

#### Phase 6: Documentation and smoke tests

- [ ] **Update README.md**: Add CI workflow example, document `--post` and `--new-post` flags
- [ ] **Update `docs/guide.md`**: Add posting configuration section, permissions reference
- [ ] **Expand smoke tests** in `scripts/smoke-test.sh`:
  - `--post --help` shows flag in help text
  - `--new-post --help` shows flag in help text
  - `--post` without config shows clear error for bulk commands
  - `--post` with single issue (can't easily test posting in smoke tests — test the error path)
  - Preflight JSON includes `posting_readiness` fields
  - CI logging format (set `GITHUB_ACTIONS=true`, check for `::notice::` in stderr)
- [ ] **Add example workflow file**: `docs/examples/velocity-report.yml`

## Acceptance Criteria

### Functional Requirements

- [ ] `gh velocity flow lead-time 42 --post` creates/updates a comment on issue #42
- [ ] `gh velocity report --since 30d --post` creates/updates a Discussion
- [ ] Repeated `--post` runs update in place (idempotent via markers)
- [ ] `--new-post` creates a new comment/Discussion regardless
- [ ] `--post` writes markdown to stdout AND posts to GitHub
- [ ] `--post` without `discussions.category_id` returns clear error for bulk commands
- [ ] `--post` with missing permissions returns error with exact YAML fix
- [ ] `--post -f json` wraps JSON in code fence when posting
- [ ] Preflight JSON includes `posting_readiness` with `discussions_enabled`, `has_issues_write`, `has_discussions_write`, `category_valid`
- [ ] Stderr uses `::error::`/`::warning::`/`::notice::` when `GITHUB_ACTIONS=true`

### Non-Functional Requirements

- [ ] All existing tests continue to pass
- [ ] No behavior changes when `--post` is not used
- [ ] Posting adds at most 2 API calls per invocation (search + create/update)
- [ ] Discussion search limited to 50 most recent (bounded performance)

### Quality Gates

- [ ] Table-driven tests for marker parsing, posting logic, and CI logging
- [ ] Smoke tests cover error paths and flag combinations
- [ ] `task quality` passes (lint + staticcheck + all tests)

## Dependencies & Risks

| Risk | Mitigation |
| --- | --- |
| First write operation — mutation bugs could spam issues | Test with a dedicated test repo; `--new-post` is opt-in |
| Fine-grained PATs don't expose scopes | Preflight checks are best-effort; document limitation |
| Concurrent CI runners may create duplicates | Document as known limitation; last-write-wins on retry |
| Discussion search at scale | Limit to 50 most recent; document behavior |
| Locked issues reject comments | Catch 403, return `POST_FAILED` with helpful message |

## Design Decisions

| Decision | Rationale |
| --- | --- |
| Markers = entire body | Simpler replacement algorithm; no partial-body parsing |
| One comment per metric per issue | Multiple metrics get separate comments; clear separation |
| `--new-post` implies `--post` | Avoids silent no-op when user forgets `--post` |
| `--post` coerces to markdown | Pretty format contains ANSI escapes; markdown is the natural posting format |
| Only `GITHUB_ACTIONS` env var | Standard variable set by Actions; no custom override needed |
| Fail at post time (no early check) | Simpler; hint at `config preflight` for pre-validation |
| Search 50 most recent discussions | Bounded performance; covers active repos |

## References

### Internal

- Brainstorm: `docs/brainstorms/2026-03-10-github-actions-and-posting-brainstorm.md`
- Client struct: `internal/github/client.go:10-16`
- GraphQL pattern: `internal/github/discover.go:56-98`
- Error codes: `internal/model/errors.go:9-16`
- Config: `internal/config/config.go:96-98` (DiscussionsConfig)
- Preflight: `cmd/preflight.go:112-127` (PreflightResult)
- Format pipeline: `cmd/report.go:97-105` (format dispatch pattern)
- Existing `--post` flag: `cmd/root.go:203-204`

### External

- GitHub REST API: Issue Comments — `POST /repos/{owner}/{repo}/issues/{number}/comments`
- GitHub GraphQL: `createDiscussion` mutation
- GitHub GraphQL: `updateDiscussion` mutation
- GitHub Actions workflow commands: `::error::`, `::warning::`, `::notice::`
