---
date: 2026-03-20
topic: terminology-and-orthogonality
---

# Terminology and Orthogonality

## Problem Frame

gh-velocity has three independent axes — **scope/lifecycle**, **iteration**, and **effort** — but the docs don't explicitly name them as orthogonal. Since the project board was removed from lifecycle, the iteration and effort strategy pages can imply a tighter board coupling than actually exists. There is no canonical glossary, and the same concepts are re-explained in multiple places instead of defined once and linked.

Additionally, when an issue is in scope but not on the project board, the `numeric` and `field:` effort strategies silently treat it as "not assessed" with no insight surfaced to the user.

## Requirements

- R1. **Canonical definitions page** — Add a concepts page (`site/content/concepts/terminology.md`) with concise definitions for: Scope, Lifecycle, Iteration, Effort. Each definition is 1–3 sentences. This is the single source of truth; other pages link here instead of re-explaining.
- R2. **velocity-setup guide reframe** — Rewrite the intro of `velocity-setup.md` to explicitly state that iteration and effort are orthogonal to scope/lifecycle. Reference the terminology page. Trim any redundant definitions.
- R3. **labels-vs-board.md link** — Update the "What project boards do" section to link to the terminology definitions rather than re-explaining iteration and effort roles inline.
- R4. **Code comments audit** — Align godoc strings in `internal/pipeline/velocity/` (effort.go, period.go, velocity.go) and `internal/config/config.go` to use the canonical terms consistently. No identifier renames.
- R5. **Effort insight for missing board items** — When effort strategy is `numeric` or uses `field:` matchers and N items in scope are not on the project board, surface an insight: "X issues are not on the project board and have no effort assigned" **with the list of issue numbers**. GitHub search cannot filter "not on board", so this is the only way users can discover which items are missing. Items remain "not assessed".
- R6. **Unit tests for effort edge case** — Test that `NumericEvaluator` and `AttributeEvaluator` (with field matchers) correctly produce "not assessed" for items missing from the board, and that the insight includes the specific issue numbers.

## Success Criteria

- A user reading the concepts page can understand what each axis means without reading any other page
- The velocity-setup guide no longer re-defines terms — it links to concepts/terminology
- `labels-vs-board.md` links to terminology instead of re-explaining
- Code comments in velocity pipeline use the same terms as the docs
- Running velocity with `numeric` effort on a mixed scope (some items on board, some not) produces the insight message

## Scope Boundaries

- No identifier renames in code (only comments/godoc)
- No changes to lifecycle or scope filtering behavior
- No changes to effort calculation logic (items remain "not assessed")
- No new config fields
- Documentation should get shorter, not longer — define once, link everywhere

## Key Decisions

- **Definitions live in concepts/terminology.md**: Single source of truth, linked from guides and other concept pages
- **Insight, not warning**: Missing-board items surface as an insight (informational) not a warning (actionable problem)
- **Docs + code comments, not full rename**: Align terminology in docs and godoc without renaming identifiers

## Deferred to Planning

- [Affects R4][Needs research] Identify all godoc strings in velocity pipeline that use inconsistent terminology
- [Affects R5][Technical] Determine where in the pipeline the "not on board" insight should be generated and how it flows to output

## Next Steps

→ `/ce:plan` for structured implementation planning
