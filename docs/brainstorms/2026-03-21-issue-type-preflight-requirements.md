---
date: 2026-03-21
topic: issue-type-preflight-matchers
---

# Preflight: Emit type: Matchers from Discovered Issue Types

## Problem Frame

Preflight discovers GitHub Issue Types via `ListIssueTypes()` and reports them in hints, but never emits `type:` matchers in the generated `--write` config. Root cause: evidence probing runs `type:` matchers against REST-fetched issues which lack the `IssueType` field (GraphQL-only), so hit counts are always 0 and the rendering code filters them out.

Repos that use GitHub Issue Types (Bug, Feature, Task, etc.) get configs with only label and title matchers — missing the strongest classification signal available.

## Requirements

- R1. When preflight discovers issue types that map to categories via `typePatterns`, emit `type:<Name>` matchers in the generated config without requiring an additional API call.
- R2. `type:` matchers should appear **before** label matchers in each category's match list (strongest signal first, first-match-wins).
- R3. Evidence output should indicate these matchers are repo-configured (e.g., count annotation or marker) rather than showing misleading 0-hit counts.
- R4. Unmapped discovered types should still produce the existing hint ("add type:X to a category if desired") — no behavior change there.

## Success Criteria

- Running `gh velocity config preflight` on a repo with GitHub Issue Types produces a config that includes `type:` matchers in the appropriate categories.
- The `type:` matchers appear before label matchers in each category.
- No additional API calls are made beyond the existing `ListIssueTypes()`.

## Scope Boundaries

- Preflight config generation only — not changing the general `SearchIssues` path or other commands.
- Not adding new type-to-category mappings beyond the existing `typePatterns` map.
- Not changing how the classify package or other commands consume `IssueType` data.

## Key Decisions

- **Trust repo-configured types as sufficient signal**: If `ListIssueTypes()` returns types that match `typePatterns`, that's enough to emit matchers. No need to fetch IssueType per-issue for evidence hit counts.
- **type: before label: in match order**: Issue types are the repo owner's authoritative categorization, stronger than labels which can be applied inconsistently.

## Outstanding Questions

### Deferred to Planning

- [Affects R3][Technical] How to represent "repo-configured" evidence in the `MatcherEvidence` struct — could use a new field, a sentinel count value, or a separate rendering path.

## Next Steps

→ `/ce:plan` for structured implementation planning
