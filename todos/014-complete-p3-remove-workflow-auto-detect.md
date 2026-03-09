---
status: pending
priority: p3
issue_id: 014
tags: [code-review, simplicity, config]
dependencies: []
---

# Consider Removing Workflow Auto-Detection

## Problem Statement

The `workflow: auto` mode makes an API call on every invocation to determine something the user already knows. Most repos will set the config once and forget it.

**Raised by:** Code Simplicity Reviewer

## Proposed Solutions

### Option A: Default to `pr`, remove auto-detection
- Users who work locally set `workflow: local` in config
- Removes detection logic and its edge cases
- Removes an API call per invocation
- **Effort:** Saves small effort
- **Risk:** Low

### Option B: Keep auto-detection
- Better first-run experience
- **Risk:** Low (just adds complexity)

## Recommended Action

Decision for plan author. Auto-detect is a nice UX touch but adds complexity.
