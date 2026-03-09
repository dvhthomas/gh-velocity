---
status: complete
priority: p3
issue_id: "037"
tags: [code-review, agent-native, consistency]
dependencies: []
---

# Unify warnings: include in JSON output for all commands

## Problem Statement

Warnings are handled inconsistently across commands:
- `release` command includes warnings in JSON output via `WriteReleaseJSON`
- `cycle-time` writes warnings to stderr even in JSON mode
- `lead-time` never passes warnings at all
- API fallback warning goes to stderr only

Agents consuming JSON output miss important context when warnings are only on stderr.

**Raised by:** Agent-Native Reviewer (WARNING)

## Proposed Solutions

### Option A: Include warnings array in all JSON outputs (Recommended)
- All JSON output structs already have `Warnings []string` fields
- Ensure all commands pass their warnings through to the JSON writer
- Keep stderr warnings for pretty/markdown modes
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] cycle-time JSON output includes warnings array
- [ ] lead-time JSON output includes warnings array when applicable
- [ ] Pretty/markdown still output warnings to stderr
- [ ] All tests pass
