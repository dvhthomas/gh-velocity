---
date: 2026-03-21
topic: issue-type-enrichment-for-rest-search
---

# Enrich REST-Sourced Issues with IssueType for type: Matchers

## Problem Frame

`SearchIssues()` uses the REST Search API, which does not return GitHub Issue Types. Commands that classify issues (report quality, velocity, throughput, WIP, cycle time, lead time) use REST search, so `type:` matchers in the config silently fail — `IssueType` is always empty. The label/title fallback catches most cases, but repos relying solely on issue types for classification get zero matches.

This gap was exposed by the preflight change that now emits `type:` matchers in generated configs. The release command already works (uses GraphQL `FetchIssues`), but all other metric commands do not.

## Requirements

- R1. Add a lightweight `EnrichIssueTypes(ctx, issues)` method on the GitHub client that batch-fetches only `number` + `issueType { name }` via GraphQL and merges results back into the issue slice.
- R2. Add `HasTypeMatchers() bool` to `classify.Classifier` so callers can cheaply check whether enrichment is needed.
- R3. Each pipeline that searches issues and classifies them should call `EnrichIssueTypes()` when `HasTypeMatchers()` returns true.
- R4. The enrichment must be gated — no extra API calls when the config has no `type:` matchers.
- R5. Batch size should match existing patterns (20 per query via GraphQL aliases).

## Success Criteria

- A repo with `type:Bug` as its only bug matcher correctly classifies bug issues in report quality, velocity, throughput, and other REST-sourced commands.
- No additional API calls when config only uses `label:` and `title:` matchers.
- Enrichment is cached per-process like other GraphQL calls.

## Scope Boundaries

- Preflight is not affected (it uses its own discovery path, already shipped).
- Release command is not affected (already uses GraphQL `FetchIssues`).
- Not changing `SearchIssues()` itself — enrichment is opt-in at the caller level.
- Not enriching PRs — `IssueType` is an issue-only field.

## Key Decisions

- **Lightweight query over reusing FetchIssues**: New query fetches only `number` + `issueType`, not full issue data. Minimizes API cost.
- **Classifier-aware gating**: `HasTypeMatchers()` on `Classifier` lets each pipeline decide. Explicit, no hidden cost.
- **Per-pipeline call site**: Each pipeline calls enrichment after its search. No wrapper magic.

## Affected Pipelines

| Pipeline | File | Search Call | Classifies? |
|---|---|---|---|
| report quality | `cmd/report.go:670` | `SearchIssues` | Yes |
| velocity | `internal/pipeline/velocity/velocity.go:182,260` | `SearchIssues` | Yes |
| throughput | `internal/pipeline/throughput/throughput.go:52,84` | `SearchIssues` | Yes (via categories) |
| WIP | `internal/pipeline/wip/wip.go:98` | `SearchIssues` | Yes |
| cycle time | `internal/pipeline/cycletime/cycletime.go:175` | `SearchIssues` | Yes (lifecycle matchers) |
| lead time | `internal/pipeline/leadtime/leadtime.go:109` | `SearchIssues` | Yes |
| issue detail | `internal/pipeline/issue/issue.go:89` | `SearchIssues` | Yes |
| config validate | `cmd/config_validate_velocity.go:156` | `SearchIssues` | Yes |

## Outstanding Questions

### Deferred to Planning

- [Affects R1][Technical] Should enrichment results be cached separately or share the existing cache infrastructure? The cache key would be different from SearchIssues since it's a different query shape.
- [Affects R3][Technical] For pipelines that search multiple times (throughput has 3 search calls), should enrichment happen once on the merged result or per-search?
- [Affects R2][Technical] Should `HasTypeMatchers()` also cover lifecycle matchers that use `type:` syntax, or only quality category matchers?

## Next Steps

→ `/ce:plan` for structured implementation planning
