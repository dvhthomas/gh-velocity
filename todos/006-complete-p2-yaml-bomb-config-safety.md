---
status: complete
priority: p2
issue_id: 006
tags: [code-review, security, yaml, config]
dependencies: []
---

# YAML Bomb Protection and Config Validation

## Problem Statement

The config file lives in the repo and is therefore attacker-controlled. A maliciously crafted `.gh-velocity.yml` could cause memory exhaustion via YAML anchors/aliases (billion laughs), or inject unexpected values into API calls. The deepened plan notes NaN/Inf guards but doesn't address other YAML safety issues.

**Raised by:** Security Sentinel (MEDIUM)

## Findings

- yaml.v3 supports anchors and aliases — exponential expansion possible
- No file size cap specified
- Config values (`project.id`, `category_id`) flow directly into API calls
- The plan says "Ignore unknown keys (forward-compatible)" — Security argues this should at least warn

## Proposed Solutions

### Option A: Size cap + strict validation (Recommended)
- Read config with `io.LimitReader` capped at 64KB
- Validate all parsed values: `hotfix_window_hours` is positive integer in range, GraphQL node IDs match patterns like `^PVT_[a-zA-Z0-9]+$`
- Warn on unknown keys to stderr (don't fail — forward-compatible)
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] Config file read with 64KB size cap
- [ ] All config values validated after parsing
- [ ] Unknown keys produce stderr warning
- [ ] NaN/Inf guards on any float64 config fields
