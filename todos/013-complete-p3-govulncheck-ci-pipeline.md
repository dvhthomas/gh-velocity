---
status: complete
priority: p3
issue_id: 013
tags: [code-review, security, ci, dependencies]
dependencies: []
---

# Add govulncheck to Quality Pipeline

## Problem Statement

The plan does not mention dependency vulnerability scanning. Three dependencies (go-gh, cobra, yaml.v3) plus their transitive deps should be checked for known vulnerabilities.

**Raised by:** Security Sentinel (LOW)

## Proposed Solutions

### Option A: Add govulncheck to `task quality` (Recommended)
- `go install golang.org/x/vuln/cmd/govulncheck@latest`
- Add to Taskfile: `govulncheck ./...`
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] `govulncheck` runs in `task quality`
- [ ] CI fails on known vulnerabilities
