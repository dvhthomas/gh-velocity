---
status: complete
priority: p2
issue_id: "033"
tags: [code-review, performance]
dependencies: []
---

# Config parses YAML twice (Load + warnUnknownKeys)

## Problem Statement

`config.Load()` calls `yaml.Unmarshal` into the Config struct, then `warnUnknownKeys` calls `yaml.Unmarshal` again into a `map[string]any`. This double parse is unnecessary.

**Raised by:** Performance Oracle (SIGNIFICANT)

## Findings

- `config.go:91` — first `yaml.Unmarshal(data, cfg)`
- `config.go:96` calls `warnUnknownKeys(data)` which at line 143 does `yaml.Unmarshal(data, &raw)`
- Two parses of the same data for every config load

## Proposed Solutions

### Option A: Single parse into map, then re-marshal or decode (Recommended)
- Parse once into `map[string]any`, check unknown keys, then use `yaml.Decoder` with `KnownFields(true)` to decode into struct
- Or: use `yaml.Decoder` with `KnownFields(true)` which does both in one pass
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] Config YAML is parsed only once
- [ ] Unknown key warnings still work
- [ ] All config tests pass

## Work Log

### 2026-03-09 - Created from code review
**By:** Review synthesis
