---
status: complete
priority: p2
issue_id: 022
tags: [code-review, architecture, reliability]
dependencies: []
---

# Propagate Context to go-gh REST Client Calls

## Problem Statement

Every method in `internal/github/` accepts `context.Context` but discards it. The `c.rest.Get(path, &resp)` call doesn't pass context. This means Ctrl+C cancellation and timeouts don't propagate to API calls — the CLI hangs until the HTTP request completes.

**Raised by:** Architecture Strategist, Pattern Recognition Specialist

## Resolution

Used go-gh v2's `RESTClient.DoWithContext(ctx, method, path, body, response)` method to propagate
the `context.Context` parameter through to all HTTP calls.

### Changes

- `internal/github/issues.go` — replaced `c.rest.Get(path, &resp)` with `c.rest.DoWithContext(ctx, "GET", path, nil, &resp)`
- `internal/github/releases.go` — same replacement

Note: `internal/github/commits.go` was listed in the original findings but does not exist in the codebase.

## Acceptance Criteria

- [x] `ctx` parameter is passed through to HTTP calls where possible
- [x] Ctrl+C during an API call terminates the request promptly
