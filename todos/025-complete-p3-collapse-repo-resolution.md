---
status: complete
priority: p3
issue_id: 025
tags: [code-review, simplicity]
dependencies: []
---

# Collapse Repo Resolution Indirection

## Problem Statement

`cmd/root.go` has 5 functions + 1 struct (70 lines) for repo resolution: `resolveRepo` → `currentRepo` → `detectGhRepo` → `ghrepo` → `parseRepoFromEnv` → `parseRepoFlag`. Two middle functions (`currentRepo`, `detectGhRepo`) are pure pass-throughs.

**Raised by:** Code Simplicity Reviewer, Pattern Recognition Specialist

## Resolution

Used **Option A** (go-gh's built-in repo resolution). Replaced 5 functions + 1 struct (70 lines) with a single `resolveRepo` function (25 lines) that delegates to `github.com/cli/go-gh/v2/pkg/repository`:

- `repository.Parse()` handles `--repo` flag and `GH_REPO` env parsing
- `repository.Current()` handles git remote detection (new capability not previously working)
- Deleted: `repoInfo` struct, `parseRepoFlag`, `currentRepo`, `detectGhRepo`, `ghrepo`, `parseRepoFromEnv`

## Acceptance Criteria

- [x] Investigate go-gh's repo resolution before writing custom code
- [x] Use go-gh's built-in resolution if it covers our needs
- [x] If custom code needed, repo resolution is ≤ 20 lines
- [x] `repoInfo` struct removed
