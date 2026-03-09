---
status: complete
priority: p3
issue_id: "036"
tags: [code-review, simplicity]
dependencies: []
---

# Simplify redundant config loading branches in root.go

## Problem Statement

`cmd/root.go:90-103` has two nearly identical branches for config loading — both call `config.Load(config.DefaultConfigFile)`. The only difference is the `!hasLocal` branch falls back to `Defaults()` on error. This can be simplified.

**Raised by:** Architecture Strategist, Code Simplicity Reviewer

## Proposed Solutions

### Option A: Single load with fallback (Recommended)
```go
cfg, err := config.Load(config.DefaultConfigFile)
if err != nil && !hasLocal {
    cfg = config.Defaults()
} else if err != nil {
    return err
}
```
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] Config loading is a single code path
- [ ] Behavior unchanged: local repo requires valid config, remote-only falls back to defaults
- [ ] All tests pass
