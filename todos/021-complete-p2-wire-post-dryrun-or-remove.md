---
status: complete
priority: p2
issue_id: 021
tags: [code-review, architecture, yagni]
dependencies: []
---

# Wire --post Flag, Remove --dry-run

## Problem Statement

The `--post` and `--dry-run` flags are registered as persistent flags and stored in `Deps`, but no subcommand reads `deps.Post` or `deps.DryRun`. The `internal/posting/` package exists but is never imported by any command. An agent or user passing `--post` gets silent success with nothing posted.

**Raised by:** Code Simplicity Reviewer, Agent-Native Reviewer

## Findings

- `cmd/root.go:27-28` — `DryRun` and `Post` fields in Deps
- `cmd/root.go:103-104` — flags registered
- `cmd/release.go`, `cmd/leadtime.go`, `cmd/cycletime.go` — none read deps.Post
- `internal/posting/poster.go` — stub with TODO, never imported

## Design Decision

Default behavior is read-only (preview/console output). `--post` explicitly opts in to writing. This makes `--dry-run` unnecessary — absence of `--post` IS the dry run.

## Proposed Solutions

### Option A: Keep --post, remove --dry-run (Approved)
- Remove `--dry-run` flag and `Deps.DryRun` — not needed since default is read-only
- Keep `--post` as explicit opt-in for write operations
- Wire `deps.Post` to actually trigger posting when implemented
- Delete `internal/posting/` stub until posting is implemented (Phase 3)
- Until posting is wired: `--post` returns clear error "posting not yet implemented"
- **Effort:** Small
- **Risk:** None

## Acceptance Criteria

- [ ] `--dry-run` flag removed entirely
- [ ] Default behavior is read-only (no `--post` = preview only)
- [ ] `--post` is the explicit opt-in for write operations
- [ ] `--post` returns clear error until posting is implemented
- [ ] No flags exist that are silently ignored
