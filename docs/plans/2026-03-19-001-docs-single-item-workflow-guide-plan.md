---
title: "docs: Single-item workflow guide"
type: feat
status: completed
date: 2026-03-19
origin: docs/brainstorms/2026-03-19-single-item-workflow-docs-requirements.md
---

# docs: Single-item workflow guide

## Overview

Write documentation showing users how to wire up `.gh-velocity.yml` + a GitHub Actions workflow to automatically post lead-time and cycle-time comments on individual issues and PRs when they close/merge. This fills a gap — existing docs only cover bulk reporting to Discussions.

## Problem Statement / Motivation

Users who want "set and forget" per-item metrics have no guide. The existing CI section in `docs/guide.md` (line 624) covers weekly reports, release metrics, and trend reports — all bulk operations. The single-item `--post` capability exists but is undocumented as a CI workflow.

## Proposed Solution

Two deliverables:
1. **Standalone guide** at `docs/single-item-workflow.md` — end-to-end walkthrough
2. **Cross-reference** added to `docs/guide.md` CI section — brief pointer to the new guide

### Guide Structure

```
# Automatic metrics on issues and PRs

## Quick start (existing users) ← skip-ahead anchor
  → Points directly to "The workflow" section

## Prerequisites
  - gh-velocity installed
  - .gh-velocity.yml in repo root

## Configuration
  - Minimal config example (R6)
  - What config create generates vs what you actually need
  - Link to full config reference in guide.md

## The workflow (R3, R4, R7)
  - Complete copy-pasteable YAML
  - Line-by-line explanation of trigger filtering
  - Permissions block explained

## How posting works (R5)
  - --post flag and dry-run safety
  - GH_VELOCITY_POST_LIVE=true
  - Idempotent updates (reclose = update, not duplicate)
  - Brief, links to site/content/guides/posting-reports.md for details

## What to expect
  - Two comments on two items when "Closes #N" PR merges
  - Fork PRs may not receive comments (GITHUB_TOKEN restriction)
  - Concurrent runs are safe (no shared state)

## Testing locally
  - Run lead-time <N> --post without GH_VELOCITY_POST_LIVE (dry-run preview)
  - Run with GH_VELOCITY_POST_LIVE=true to post live
  - Then commit the workflow file

## Troubleshooting
  - Comment not appearing → check GH_VELOCITY_POST_LIVE, permissions, --debug
  - state_reason null → explain when this happens
  - Link to main troubleshooting in guide.md
```

## Technical Considerations

### Workflow YAML specifics

**Permissions block** (surfaced by SpecFlow — `pull-requests: write` is required for PR comments):

```yaml
permissions:
  contents: read
  issues: write
  pull-requests: write
```

**Trigger filtering:**

```yaml
on:
  issues:
    types: [closed]
  pull_request:
    types: [closed]
```

Issue job condition: `if: github.event_name == 'issues' && github.event.issue.state_reason == 'completed'`
PR job condition: `if: github.event_name == 'pull_request' && github.event.pull_request.merged == true`

**`state_reason` edge case:** Issues closed via older API integrations or some bots may have `state_reason: null`. The strict `== 'completed'` check skips these. Document this trade-off; users can remove the condition if they want all closures to trigger.

**Installation:** Use `gh extension install dvhthomas/gh-velocity` (established CI pattern, goreleaser publishes cross-platform binaries). No Go build needed.

**Format flag:** Use `--post` without `--results` flag. The `--post` flag coerces to markdown automatically. Do NOT combine with `--results json` (would post a JSON blob as a comment).

**`workflow_dispatch`:** Include for manual testing from the Actions tab.

### Minimal config (R6)

From code analysis (`internal/config/config.go`): the config file must exist, but all fields are optional. Defaults provide `cycle_time.strategy: issue` and basic bug/feature categories.

For the two single-item commands:
- `lead-time <N>` — needs only the file to exist (computes created→closed)
- `cycle-time --pr <N>` — needs only the file to exist (computes created→merged)

Show a practical minimum with categories (users will likely add `report` later):

```yaml
quality:
  categories:
    - name: bug
      match:
        - "label:bug"
    - name: feature
      match:
        - "label:enhancement"
```

Optionally suggest running `gh velocity config preflight --write` to auto-detect categories from repo data.

### Dual-trigger scenario

When a PR with "Closes #42" merges, GitHub fires both `issues[closed]` and `pull_request[closed]`. Both workflow jobs run. This is correct: lead-time goes on issue #42, cycle-time goes on the PR. Document this explicitly so users understand the two workflow runs.

### Fork PRs

`GITHUB_TOKEN` for fork PRs has reduced write permissions. Cycle-time comments may fail silently on fork PRs. Note this without solving it — `pull_request_target` has security implications and is out of scope.

## Acceptance Criteria

- [ ] `docs/single-item-workflow.md` exists with all sections from the guide structure above
- [ ] Contains a complete, copy-pasteable GitHub Actions workflow YAML that only requires changing `owner/repo` (R7)
- [ ] New user can follow the guide end-to-end in ~15 minutes (R1, R2)
- [ ] Existing user can skip to "The workflow" section via anchor (R2)
- [ ] Workflow YAML includes correct `permissions:` block with `pull-requests: write` (R3)
- [ ] Trigger conditions filter to `completed` issues and `merged` PRs (R4)
- [ ] `--post`, dry-run, `GH_VELOCITY_POST_LIVE`, and idempotent updates are explained (R5)
- [ ] Minimal config example included (R6)
- [ ] `docs/guide.md` has a cross-reference pointing to the new guide (R8)
- [ ] Dual-trigger scenario ("Closes #N" PR merge) is documented
- [ ] Fork PR limitation is mentioned
- [ ] `workflow_dispatch` included for manual testing
- [ ] No duplication of posting concepts already in `site/content/guides/posting-reports.md` — cross-reference instead

## Files to Create/Modify

| File | Action | Notes |
|------|--------|-------|
| `docs/single-item-workflow.md` | Create | Main deliverable — standalone guide |
| `docs/guide.md` | Edit (line ~840) | Add cross-reference after "Scheduled trend reports" section, before "How-to recipes" |

## Dependencies & Risks

- **Dependency:** `lead-time <N> --post` and `cycle-time --pr <N> --post` must work as currently implemented. No code changes required.
- **Risk:** The `state_reason` condition may surprise users whose bots close issues without setting it. Mitigated by documenting the behavior and showing how to remove the condition.
- **Risk:** Fork PR limitation. Mitigated by documenting it as a known GitHub Actions restriction.

## Sources & References

### Origin

- **Origin document:** [docs/brainstorms/2026-03-19-single-item-workflow-docs-requirements.md](docs/brainstorms/2026-03-19-single-item-workflow-docs-requirements.md) — Key decisions: one workflow file with two triggers, layered audience, standalone doc + cross-reference, lead-time and cycle-time only.

### Internal References

- Existing CI section: `docs/guide.md:624`
- Posting implementation: `cmd/post.go`
- Dry-run logic: `cmd/root.go:302`
- Config defaults: `internal/config/config.go:210`
- Default config template: `cmd/config.go:165`
- Existing workflow example: `.github/workflows/velocity.yaml`
- Posting guide: `site/content/guides/posting-reports.md`
- CI setup guide: `site/content/getting-started/ci-setup.md`
- Example configs: `docs/examples/`
