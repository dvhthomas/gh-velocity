---
status: complete
priority: p3
issue_id: 028
tags: [code-review, performance, data-integrity]
dependencies: [026]
---

# Paginate ListReleases (Currently Capped at 100)

## Resolution

This todo is **moot**. The `ListReleases` function was removed in todo #026 (unused code removal). Only `GetRelease` (single release by tag) remains in `internal/github/releases.go`, which does not have a pagination concern.

No code changes required.

## Original Problem Statement

`ListReleases` hardcoded `per_page=100` with no pagination. Repos with 100+ releases would silently lose data.

**Raised by:** Performance Oracle, Architecture Strategist
