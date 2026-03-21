# gh-velocity Development Guide

A Go-based GitHub CLI extension for velocity and quality metrics.

## Quick Start

```bash
task build       # Build the binary
task test        # Run all tests
task quality     # Lint + staticcheck and integration tests
task test:coverage  # Tests with coverage report
```

## Conventions

- **Go only.** No shell scripts for core logic.
- **Table-driven tests** for all metric calculations and linking heuristics.
- **Narrow interfaces** defined at consumer site, not provider. E.g., `IssueQuerier` in the package that needs it.
- **`context.Context`** propagated from root command through all API and git calls.
- **GraphQL variables only** — never use `fmt.Sprintf` to build GraphQL queries. Always use the `variables` map in `client.Do(query, variables, &result)`. User/config values (`project.id`, `status_field_id`, etc.) must only enter queries as GraphQL variables.
- **Sequential `cmds`** in Taskfile, not parallel `deps` (prevents race conditions).
- **Integration tests** run against the built binary (`task build`), not `go run`.
- **Scope-first data fetching** — all issue/PR data fetching must go through GitHub Search API (REST) or GraphQL project items queries with a pre-assembled query string. Never hardcode `repo:`, `is:issue`, or date qualifiers directly in API calls. Instead, assemble queries from user scope (config + `--scope` flag) and command lifecycle qualifiers via `internal/scope`. This ensures every command benefits from user-defined filtering, even commands like `quality release` that only need scope (not lifecycle).
- Design software for both humans (easy, obvious) and for machines (clear JSON output with breadcrumbs).

## Hard Rules

The following rules must **NEVER** be broken:

- Only a human can run an update of live GitHub data using the `GH_VELOCITY_POST_LIVE` environment. An agent or script may *NEVER* under any circumstances run such a command, or run code that has a similar effect:

  ```sh
  # Post lead time as a comment on issue #42
  GH_VELOCITY_POST_LIVE=true gh velocity flow lead-time 42 --post
  ```

- **Project board workflow is mandatory.** All work must follow the `github-project-workflow` skill. The [project board](https://github.com/users/dvhthomas/projects/1) tracks issue lifecycle (Backlog → Ready → In Progress → In Review → Done). Skipping transitions corrupts velocity metrics. Only use the `gh` CLI for GitHub interactions.

- **Always use GitHub worktrees** for local development. Do not clone the repository directly — use `gh repo clone --worktree` instead.

## Dependencies

- See go.mod
- GitHub `gh` CLI
- https://taskfile.dev/ is the Go-based task runner.

## Output Flags

- `--results`/`-r` — output format(s): `pretty` (default), `json`, `markdown`, `html`. Comma-separated for multiple: `--results md,json,html`.
- `--write-to <dir>` — write all result formats as files to this directory (silences stdout). Required when using multiple `--results` formats. `pretty` is disallowed with `--write-to`.
- `--post` requires markdown in `--results`. Posts use an independent buffer, decoupled from stdout and `--write-to`.
- Warnings appear in both stderr (when not suppressed) and in result JSON `warnings` arrays. When `--results json` is the sole format going to stdout, stderr warnings are suppressed to preserve JSON stream purity. When `--write-to` is set, stderr is always active.
- `--debug` always goes to stderr regardless of warning suppression.

## Error Handling

Exit codes: 0=success, 1=general, 2=config, 3=auth, 4=not found.
When `--results json`, errors are also JSON: `{"error": "message", "code": N}`.

## Quality

- Run `task quality` before every commit. Run `task test` continuously during development.
- Prove the existence of the bug before fixing it: jumping to solutions is an anti-pattern. Tests (unit, integration) are the preferred way of (dis)proving a hypothesis because they catch regressions and cumulatively build confidence.

### Testing tiers

| Tier | Command | Cost | When |
|------|---------|------|------|
| Unit tests | `task test` | Zero API calls | Every change |
| Lint + static analysis | `task quality` | Zero API calls | Before every commit |
| Smoke tests | `task test:smoke` | ~50 API calls | Before PRs |
| Pre-release validation | `task release:preflight` | ~50 API calls | Before tagging a release |
| Showcase (full) | `task showcase` | **Thousands** of API calls | CI cron only (daily) |
| Showcase (single repo) | `task showcase -- --dry-run "GitHub CLI"` | ~100 API calls | Manual, when testing preflight heuristics |

**Do not run `task showcase` locally without `--dry-run` and a repo filter.** The full showcase runs preflight + report against 11+ repos and posts to Discussions. It is designed for the daily CI cron job, not for local development. Use `--dry-run` to skip posting, and pass a repo name to test a single project.
