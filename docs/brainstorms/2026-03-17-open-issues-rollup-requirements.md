---
date: 2026-03-17
topic: open-issues-rollup
---

# Open Issues Rollup — Stability-First Sequencing

## Problem Frame

gh-velocity has 8 open issues spanning bugs, features, and chores. Without a deliberate sequencing, feature work risks building on a shaky foundation (undetected report bugs, wrong module path, unverified first-run experience). A stability-first approach ensures each phase builds on solid ground.

## Requirements

- R1. Complete all 8 open issues (#78, #84, #86, #85, #97, #105, #108, #109) in dependency order
- R2. Module path rename (#78) ships first as a standalone PR to avoid merge conflicts with subsequent work
- R3. Bugs (#108, #97) and verification (#84) ship before any new features
- R4. Features with existing plans (#85, #86) proceed using those plans without re-planning
- R5. Each phase produces mergeable PRs — no phase depends on uncommitted work from another phase

## Phased Execution

### Phase 1 — Foundation cleanup
| Issue | Description | Scope |
|-------|-------------|-------|
| #78 | Rename Go module path `bitsbyme` → `dvhthomas` | Mechanical find-and-replace across all Go files + go.mod |

**Gate:** All tests pass with new module path.

### Phase 2 — Bug fixes + verification
| Issue | Description | Scope |
|-------|-------------|-------|
| #108 | Fix throughput detail section missing from report | Investigate `throughputOK` gate in `cmd/report.go` ~line 354; likely data flow issue between summary and detail rendering |
| #97 | Warn when config scope `repo:` conflicts with `-R` flag | Add conflict detection in `internal/scope/`; emit warning or auto-replace |
| #84 | Verify first-run experience end-to-end | 3 manual checks; may produce small follow-up fixes |

**Gate:** Report output is correct and complete. Scope conflicts are surfaced to the user.

### Phase 3 — Config enhancement
| Issue | Description | Scope |
|-------|-------------|-------|
| #105 | Configurable defect rate threshold | Add `quality.defect_rate_threshold` to config, wire through to insight generator |

**Gate:** Insight message references configured threshold; default remains 0.20.

### Phase 4 — Planned features (parallelizable)
| Issue | Description | Plan |
|-------|-------------|------|
| #85 | Separate `--results`, `--output`, `--write-to` flags | `docs/plans/2026-03-16-001-feat-results-output-writeto-flag-separation-plan.md` |
| #86 | `field:` matcher for SingleSelect effort strategy | `docs/plans/2026-03-14-001-feat-field-matcher-and-singleselect-effort-plan.md` |

These two features are independent and can be worked in parallel worktrees.

**Gate:** Both features pass their plan's acceptance criteria. Smoke tests updated.

### Phase 5 — Preflight intelligence
| Issue | Description | Scope |
|-------|-------------|-------|
| #109 | Fuzzy lifecycle label detection + pipeline discovery | Label normalization, lifecycle stage mapping, pipeline detection, strategy fallback logic |

**Gate:** Preflight detects lifecycle labels in hashicorp/terraform, github/roadmap; suggests `strategy: pr` for grafana/k6.

## Success Criteria

- All 8 issues closed with merged PRs
- `task quality` passes after each phase
- Showcase workflow produces correct output (throughput detail present, lifecycle detection improved)
- No regressions in the 76+ smoke tests

## Scope Boundaries

- No new commands or subcommands introduced
- #85 Phase 3 (`--output json` for agent diagnostics) remains deferred per its plan
- No cross-invocation cache work in this rollup (separate initiative)

## Key Decisions

- **Module rename first:** Avoids merge conflicts; the noisy diff is isolated in one PR
- **Bugs before features:** Report correctness (#108) and safety (#97) are prerequisites for trusting new feature output
- **Phase 4 parallelization:** #85 and #86 touch different subsystems, safe to develop concurrently
- **#109 last:** Largest feature; benefits from stable preflight, report, and config subsystems

## Dependencies / Assumptions

- No other branches are in flight that would conflict with the module rename
- Existing plans for #85 and #86 are still current and don't need revision
- The showcase workflow will be re-run after Phase 2 to validate bug fixes

## Outstanding Questions

### Deferred to Planning

- [Affects R3, #108][Needs research] Root cause of throughput detail absence — is `throughputOK` false, or is the data struct empty at render time?
- [Affects R3, #97][Technical] Should `-R` auto-replace `repo:` in scope query, or just warn? Issue proposes both options.
- [Affects R5, #109][Needs research] How many lifecycle keyword patterns are sufficient for good coverage without false positives?

## Next Steps

→ `/ce:plan` for structured implementation planning, starting with Phase 1
