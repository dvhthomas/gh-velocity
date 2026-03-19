---
date: 2026-03-19
topic: single-item-workflow-docs
---

# Single-Item Workflow Documentation

## Problem Frame

Users who want per-item automation — posting lead time on issue close or cycle time on PR merge — have no guide showing how to wire up `.gh-velocity.yml` + a GitHub Actions workflow for this use case. The existing docs and workflow examples focus on bulk reporting (weekly summaries to Discussions). This creates a gap for the most natural "set and forget" integration: automatic metrics comments on the items themselves as they complete.

## Requirements

- R1. Create a standalone guide (`docs/single-item-workflow.md`) covering end-to-end setup of per-item GitHub Actions automation using gh-velocity's single-item commands with `--post`.
- R2. The guide must be layered: quick-start for new users (config creation, token setup) with a "skip ahead" path for users who already have a working `.gh-velocity.yml`.
- R3. Cover two triggers in a single workflow file (`.github/workflows/velocity-item.yaml`):
  - `issues: types: [closed]` — runs `gh velocity flow lead-time <number> --post`
  - `pull_request: types: [closed]` — runs `gh velocity flow cycle-time --pr <number> --post` (only when merged)
- R4. Show the conditional logic: issue job checks `github.event.issue.state_reason == 'completed'`; PR job checks `github.event.pull_request.merged == true`.
- R5. Explain the `--post` flag, dry-run safety (`GH_VELOCITY_POST_LIVE=true`), and idempotent comment updates.
- R6. Include a minimal `.gh-velocity.yml` example tailored to this use case (scope, cycle_time strategy, categories).
- R7. Include a complete, copy-pasteable GitHub Actions workflow YAML.
- R8. Add a brief cross-reference section in `docs/guide.md` pointing to the new standalone guide.

## Success Criteria

- A user can follow the guide end-to-end and have working per-item automation within ~15 minutes.
- The workflow YAML is copy-pasteable with only `owner/repo` needing to be changed.
- Existing users can skip straight to the workflow section without re-reading config basics.

## Scope Boundaries

- Only documents the two commands that support `--post` today: `lead-time` and `cycle-time`.
- Does not cover bulk reporting, Discussions posting, or release quality.
- Does not require any new CLI features — documents existing capabilities only.
- Does not cover self-hosted runners or complex CI setups.

## Key Decisions

- **One workflow file, two triggers**: Single `.github/workflows/velocity-item.yaml` with conditional steps, rather than separate files per trigger. Keeps setup minimal.
- **Layered audience**: New users get config + token setup; existing users skip ahead. Avoids maintaining two separate guides.
- **Standalone doc + cross-reference**: Full guide in `docs/single-item-workflow.md`, brief pointer added to `docs/guide.md`.
- **Trigger filtering**: Issue close filtered to `completed` reason (not "not planned"); PR close filtered to `merged == true` (not closed-without-merge).

## Dependencies / Assumptions

- `lead-time <issue> --post` and `cycle-time --pr <N> --post` work as currently implemented.
- `GITHUB_TOKEN` in Actions has sufficient permissions to post issue/PR comments (it does by default).
- Users have `gh` CLI available in the runner (standard on `ubuntu-latest`).

## Outstanding Questions

### Deferred to Planning
- [Affects R7][Needs research] Should the workflow install gh-velocity via `go install` or download a pre-built binary from releases? Check what release artifacts exist.
- [Affects R6][Technical] What is the minimal config needed for single-item lead-time and cycle-time to work? Verify which config fields are actually required vs optional.

## Next Steps

-> `/ce:plan` for structured implementation planning
