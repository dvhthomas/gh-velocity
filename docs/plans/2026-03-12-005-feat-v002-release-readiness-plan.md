---
title: "v0.0.2 Release Readiness"
type: feat
status: completed
date: 2026-03-12
---

# v0.0.2 Release Readiness

## Overview

Ship the next release of gh-velocity so the author can install and use it on `dvhthomas/gh-velocity` and `CalcMark/go-calcmark`. This is a patch release (v0.0.2) or pre-release (v0.0.2-rc.1) — not a minor bump.

Since v0.0.1 (2026-03-11), 20+ commits landed: pipeline interface, config-required, bus-factor, reviews, my-week, actionable output, cycle-time strategy refactor, JSON warnings completeness, and docs refresh.

## Pre-Release Checklist

### Phase 1: Merge outstanding work

- [ ] Merge PR #48 (`fix/first-run-experience`) to main
- [ ] Delete the `fix/first-run-experience` and `refactor/cycle-time-strategy-rework` remote branches

### Phase 2: Fix stale smoke-test-ext.sh

`scripts/smoke-test-ext.sh` uses v0.0.1-era command names that no longer exist. Every line using `gh velocity lead-time`, `gh velocity scope`, or `gh velocity release` (without `quality` prefix) will fail against the current binary.

- [ ] Rewrite `smoke-test-ext.sh` to use current command hierarchy:
  - `gh velocity lead-time ...` → `gh velocity flow lead-time ...`
  - `gh velocity scope ...` → `gh velocity quality release --discover ...`
  - `gh velocity release ...` → `gh velocity quality release ...`
  - `gh velocity lead-time abc ...` (error test) → `gh velocity flow lead-time abc ...`
- [ ] Add missing commands to ext smoke tests: `flow cycle-time`, `flow throughput`, `risk bus-factor`, `status reviews`, `report`
- [ ] Run `task install && task test:integration` and verify all pass

### Phase 3: Fix README Quick Start vs config-required

The README says "no config needed" for Quick Start but config IS required since the config-required change. Two options:

**Option A (recommended):** Update the Quick Start to include a config step:
```bash
# Try it now against any public repo:
gh velocity config preflight -R cli/cli --write
gh velocity flow lead-time --since 30d -R cli/cli
```

**Option B:** Allow configless fallback for remote repos (`-R`). This contradicts the design decision that config is required.

- [ ] Update README Quick Start (lines 17-36) — add config creation step
- [ ] Update README CI example (lines 119-135) — add comment that `.gh-velocity.yml` must be committed
- [ ] Fix README config example (line 42): `--project 3` → `--project-url <url>` (the `--project` flag is deprecated per `cmd/preflight.go:152-153`)
- [ ] Update command tree in README (lines 55-78) to include `risk bus-factor`, `status reviews`, `status my-week`
- [ ] Remove stale config fields from README manual config section (lines 200-225):
  - `bug_labels`/`feature_labels` → replaced by `quality.categories` with matchers
  - `project.id`/`status_field_id` → replaced by `project.url`/`status_field`

### Phase 4: Verify goreleaser name template for Windows

The `.goreleaser.yml` name_template is `{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}` without `{{ .Ext }}`. goreleaser's `binary` archive format should auto-append `.exe` for Windows, but verify:

- [ ] Check that v0.0.1 GitHub Release has Windows assets with `.exe` extension. If not, add `{{ .Ext }}` to the template.

### Phase 5: Configure and validate on target repos

#### dvhthomas/gh-velocity (this repo)

- [ ] Verify `.gh-velocity.yml` root config is current (no dead fields like `lifecycle.done.query`, config fields match current schema)
- [ ] Run `gh velocity config validate` — should pass
- [ ] Run `gh velocity report --since 30d` — should produce output
- [ ] Run `gh velocity flow lead-time --since 30d` — should produce output
- [ ] Run `gh velocity status my-week --since 14d` — should produce output

#### CalcMark/go-calcmark

- [ ] Run `gh velocity config preflight -R CalcMark/go-calcmark` to see what's suggested
- [ ] The repo has a GitHub Project "CalcMark Tracker" with 26 items — preflight should detect it
- [ ] Run `gh velocity config preflight -R CalcMark/go-calcmark --project-url <project-url> --write` from inside the go-calcmark checkout
- [ ] Run `gh velocity config validate` in go-calcmark
- [ ] Run `gh velocity report --since 30d` in go-calcmark
- [ ] Run `gh velocity status my-week --since 14d` in go-calcmark

### Phase 6: Run full test suite

- [ ] `go test ./... -count=1 -race` — all pass
- [ ] `task quality` — lint + vet pass
- [ ] `task smoke` — smoke tests pass against built binary
- [ ] `task install && task test:integration` — extension smoke tests pass

### Phase 7: Tag and release

- [ ] Decide: `v0.0.2` (patch) or `v0.0.2-rc.1` (pre-release)
- [ ] `git tag v0.0.2 && git push origin v0.0.2` (goreleaser auto-triggered)
- [ ] Wait for GitHub Actions release workflow to complete
- [ ] Verify `gh extension upgrade dvhthomas/gh-velocity` picks up new version
- [ ] Verify `gh velocity version` shows correct version

### Phase 8: Post-release validation

- [ ] `gh extension install dvhthomas/gh-velocity` on a clean machine (or after `gh extension remove gh-velocity`)
- [ ] Follow the README Quick Start steps — they should work exactly as documented
- [ ] Run `gh velocity report --since 30d -R dvhthomas/gh-velocity` — should produce real output

## Acceptance Criteria

- [ ] `gh extension install dvhthomas/gh-velocity` installs the new release
- [ ] `gh velocity version` reports v0.0.2
- [ ] `gh velocity config preflight -R CalcMark/go-calcmark` works and suggests reasonable config
- [ ] README Quick Start steps work end-to-end without errors
- [ ] `smoke-test-ext.sh` passes with current command names
- [ ] JSON output from all commands includes `warnings` field

## Known non-goals for v0.0.2

- No CHANGELOG.md (goreleaser generates release notes from commits)
- No deprecation aliases for old top-level commands (`lead-time`, `scope`, etc.) — only `release` has one
- No `gh velocity version --check-update` feature
- WIP command still requires project board config (TODO for future)

## Sources

- PR #48: `fix/first-run-experience` branch (6 commits)
- `.goreleaser.yml` — release automation config
- `.github/workflows/release.yml` — triggered on `v*` tags
- `scripts/smoke-test.sh` — current (393 lines, 92+ assertions)
- `scripts/smoke-test-ext.sh` — **stale**, needs rewrite
- `README.md` — needs updates for config-required, command tree, config fields
- `docs/guide.md` — already updated in PR #48
