---
status: complete
priority: p2
issue_id: 023
tags: [code-review, architecture, agent-native]
dependencies: []
---

# Unify Warning Handling (stderr vs warnings slice)

## Problem Statement

Non-fatal issues use three different patterns: (1) `fmt.Fprintf(os.Stderr, ...)` directly, (2) `warnings = append(warnings, ...)` for formatter inclusion, (3) stderr + nil-out. In JSON mode, stderr warnings are invisible to agents parsing stdout. The `warnings` array in JSON output is the correct pattern but isn't used everywhere.

**Raised by:** Architecture Strategist, Agent-Native Reviewer, Pattern Recognition Specialist

## Findings

- `cmd/release.go:82` — stderr warning when no GitHub release exists (skips warnings slice)
- `cmd/cycletime.go:58` — stderr warning for git log failure
- `cmd/release.go:91` — correctly uses warnings slice for issue fetch failures

## Proposed Solutions

### Option A: Always use warnings slice (Recommended)
- Collect all warnings into `[]string`, pass to formatter
- Formatters render warnings in their format (stderr for pretty, JSON field for json)
- Never write directly to stderr from command handlers
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] No `fmt.Fprintf(os.Stderr, ...)` in command handlers for warnings
- [ ] All warnings appear in JSON output's `warnings` array
- [ ] Pretty/markdown formatters write warnings to stderr
