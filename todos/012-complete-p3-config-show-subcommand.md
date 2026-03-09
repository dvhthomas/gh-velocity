---
status: complete
priority: p3
issue_id: 012
tags: [code-review, agent-native, config]
dependencies: []
---

# Add `config show` and `config validate` Subcommands

## Problem Statement

An agent setting up a repo for gh-velocity has no programmatic way to check if config is valid, see resolved defaults, or determine which features are available. It would have to run a command and see if it fails.

**Raised by:** Agent-Native Reviewer (WARNING)

## Proposed Solutions

### Option A: Add config subcommands
- `gh velocity config show --format json` — outputs resolved config with defaults
- `gh velocity config validate` — checks config and returns structured errors
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] `config show` outputs resolved config in JSON
- [ ] `config validate` returns structured errors
- [ ] Both useful for agent self-diagnosis
