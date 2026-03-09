---
status: complete
priority: p2
issue_id: 010
tags: [code-review, performance, git]
dependencies: []
---

# Stream Git Output and Use Native Filtering

## Problem Statement

For repositories with large commit ranges (thousands of commits between releases), buffering all `git log` output into memory and scanning with regex is O(N) in memory. Performance degrades on large repos.

**Raised by:** Performance Oracle (HIGH)

## Findings

- `cmd.Output()` buffers everything into a single byte slice
- Squash merge commit bodies can be very large (full PR body + commit list)
- `git log --grep` can filter commits with issue references natively, much faster than scanning in Go
- `git rev-list --count` is faster than `git log` for just counting commits

## Proposed Solutions

### Option A: Stream + native filtering (Recommended)
- Use `cmd.StdoutPipe()` + `bufio.Scanner` instead of `cmd.Output()`
- Use `git log --format="%H %s"` for initial scan (hash + subject only)
- Use `git log --grep="#"` to let git pre-filter commits with issue references
- Use `git rev-list --count from..to` for commit counting
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] Git output streamed, not buffered entirely
- [ ] Minimal `--format` used for initial scans
- [ ] `git log --grep` used where applicable
- [ ] Memory usage bounded for large commit ranges
