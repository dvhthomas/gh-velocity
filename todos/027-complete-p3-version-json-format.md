---
status: complete
priority: p3
issue_id: 027
tags: [code-review, agent-native]
dependencies: []
---

# Version Command Should Respect --format json

## Problem Statement

`gh velocity version -f json` outputs plain text. An agent parsing JSON gets a parse error. The `version` command skips `PersistentPreRunE`, so `deps` is nil.

**Raised by:** Agent-Native Reviewer

## Proposed Solutions

### Option A: Parse format flag directly in version command
- Check the `--format` flag value directly (not via Deps)
- Output `{"version": "...", "build_time": "..."}` when JSON
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [x] `gh velocity version -f json` outputs valid JSON

## Resolution

Used `cmd.Flags().GetString("format")` to read the persistent `--format` flag directly in the version command's `RunE`. When `format` is `"json"`, outputs `{"build_time":"...","version":"..."}` via `encoding/json.Marshal`. Otherwise falls through to the existing plain-text output.
