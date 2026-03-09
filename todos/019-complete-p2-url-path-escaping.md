---
status: complete
priority: p2
issue_id: 019
tags: [code-review, security, api]
dependencies: []
---

# Escape URL Path Segments in GitHub API Calls

## Problem Statement

User-supplied values (`tag`, `owner`, `repo`, `base`, `head`) are interpolated directly into REST API URL paths via `fmt.Sprintf` without `url.PathEscape`. A tag containing `/`, `?`, `#`, or `%` would produce a malformed URL. While go-gh may provide some protection, relying on it is fragile.

**Raised by:** Security Sentinel

## Findings

- `internal/github/releases.go:48` — `tag` interpolated into path
- `internal/github/commits.go:27` — `base` and `head` interpolated into path
- `internal/github/issues.go:38` — `number` is int (safe)
- `owner` and `repo` from `--repo` flag also interpolated without validation

## Proposed Solutions

### Option A: URL-escape at call site (Recommended)
- Apply `url.PathEscape()` to all user-supplied path segments
- **Effort:** Small
- **Risk:** Low

### Option B: Validate at input boundary
- Reject invalid characters in `parseRepoFlag` and tag args
- **Effort:** Small
- **Risk:** Low (but doesn't protect against future call sites)

## Acceptance Criteria

- [ ] All `fmt.Sprintf` paths in `internal/github/` use `url.PathEscape` for user-supplied segments
- [ ] `parseRepoFlag` validates owner/repo against `^[a-zA-Z0-9._-]+$`
