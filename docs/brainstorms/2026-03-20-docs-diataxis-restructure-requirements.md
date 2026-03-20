---
date: 2026-03-20
topic: docs-diataxis-restructure
---

# Documentation restructure (Diataxis + 5 C's)

## Problem Frame

The docs site has good coverage but structural problems that make it hard to use. Pages mix Diataxis quadrants (how-to + explanation in one page), content is duplicated across pages, the config schema lives in Getting Started instead of Reference, and several correctness issues exist (wrong defaults, incomplete formulas). Users can't find authoritative answers because the same concept is explained differently in multiple places.

## Requirements

### Terminology (do first — prevents churn)

- R1. Standardize "PR-based" → "PR strategy" (2 instances in config.md, examples/_index.md)
- R2. Standardize "dry run" → "dry-run" (hyphenated) in all prose (7 instances across guides)
- R3. Reduce informal use of "sizing" as formal term; "effort" is the canonical term

### Structural (Diataxis alignment)

- R4. Split Interpreting Results into two clear sections: "Reading output" (how-to) and "What healthy looks like" (explanation). Can be sections on one page or two pages — but don't mix them.
- R5. Move Agent Integration JSON structure content to a JSON output reference page; keep practical jq/CI patterns in the guide.
- R6. Move config schema (120 lines) from Getting Started/Configuration to Reference/config.md. Keep only a summary ("what each section does") in Getting Started.
- R7. De-duplicate velocity effort/iteration strategy definitions — define once in reference/metrics/velocity.md, link from guides/velocity-setup.md.

### Correctness

- R8. Verify `cycle_time.strategy` default in code and fix docs to match.
- R9. Document velocity completion rate edge case: what happens when committed effort is 0.
- R10. Fix API throttle default description: "0 for local; preflight recommends 2 for CI."
- R11. Update How It Works cycle-time comparison table to mention the signal priority hierarchy from cycle-time.md reference.

### Completeness

- R12. Add all N/A reasons to Interpreting Results: no signal found, issue in backlog, negative filtered.
- R13. Add "Velocity shows high not-assessed count" to Troubleshooting.
- R14. Add "What project boards ARE used for" section to Labels vs Board (velocity iterations, effort fields).
- R15. Add 1-2 common config validation error examples to Configuration guide.

## Success Criteria

- Every page belongs to exactly one Diataxis quadrant
- No concept is defined in more than one place (single source of truth, link elsewhere)
- Terminology is consistent across all pages
- Correctness issues verified against code
- A new user can go from Getting Started index through Quick Start without hitting contradictions or dead ends

## Scope Boundaries

- Not redesigning site navigation or theme
- Not adding new pages unless splitting an existing one
- Not rewriting prose style (that was the previous docs overhaul)
- Not changing tool behavior — docs-only changes

## Key Decisions

- **Interpreting Results**: section within one page (not a split into two pages) — keeps content discoverable
- **Config schema**: move to Reference, not duplicate — Getting Started keeps a short "what each section does" summary with a link
- **Velocity de-duplication**: reference is authoritative, guide links to it

## Next Steps

→ Create tracking issue, then `/ce:work` to execute
