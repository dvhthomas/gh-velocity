---
title: "Pre-Release Analysis Skill and Release Task"
type: feat
status: abandoned
date: 2026-03-12
---

# Pre-Release Analysis Skill and Release Task

## Overview

Build a Claude Code skill (`pre-release-analysis`) that detects documentation drift, config schema mismatches, stale examples, README inaccuracies, and smoke test coverage gaps — all the nuances that accumulate between releases. Pair it with a `task release` Taskfile target that runs the full quality + analysis pipeline before tagging.

## Problem Statement

Between v0.0.1 and now, multiple documentation inconsistencies crept in unnoticed:
- README Quick Start says "no config needed" but config IS required
- README references `--project 3` (deprecated flag, should be `--project-url`)
- README command tree is missing 3 commands (`risk bus-factor`, `status reviews`, `status my-week`)
- README manual config section uses old field names (`bug_labels`, `project.id`)
- `smoke-test-ext.sh` uses v0.0.1-era command names that no longer exist
- The `.gh-velocity.yml` root config has dead fields

These are caught by human review or by running the tool and seeing failures. A skill should catch them systematically.

## Proposed Solution

### 1. Pre-Release Analysis Skill (`.claude/skills/pre-release-analysis/SKILL.md`)

A Claude Code skill that performs a comprehensive consistency audit. It checks:

#### A. README ↔ Code Consistency
- [ ] **Command tree accuracy**: Parse the command tree block in README and compare against actual Cobra commands registered in `cmd/root.go`. Flag missing or extra commands.
- [ ] **Flag documentation**: Check that flags mentioned in README (e.g., `--project N`) match actual flag definitions in `cmd/*.go` (grep for `Flags().StringVar`, `Flags().IntVar`, etc.). Flag deprecated flags documented without deprecation notes.
- [ ] **Quick Start validity**: Extract code blocks from README Quick Start section. Check each command against `--help` output to verify it would parse. Flag any command that requires config if README says "no config needed."
- [ ] **Config schema accuracy**: Compare YAML fields shown in README's manual config section against the Go struct tags in `internal/model/types.go` and `internal/config/config.go`. Flag fields that don't exist in the schema or are missing from docs.

#### B. Config Template ↔ Schema Consistency
- [ ] **`defaultConfigTemplate` validity**: The template string in `cmd/config.go` should parse as valid YAML and pass `config validate`. Run a round-trip check.
- [ ] **Example configs**: Each file in `docs/examples/*.yml` should parse without validation errors. Run `config validate` equivalent against each.
- [ ] **Root `.gh-velocity.yml`**: Should have no dead/deprecated fields. Cross-reference against config validation warnings.

#### C. Smoke Test Coverage
- [ ] **Command coverage**: List all Cobra commands from `cmd/root.go`. For each, check that at least one smoke test in `scripts/smoke-test.sh` exercises it. Flag untested commands.
- [ ] **Extension test currency**: Compare commands tested in `smoke-test-ext.sh` against current command names. Flag any that use old/removed command names.
- [ ] **Format coverage**: For each command that supports `--format`, check that smoke tests cover pretty, json, and markdown.

#### D. Guide ↔ Code Consistency
- [ ] **`docs/guide.md` config field names**: Extract YAML field references from guide and check against schema structs.
- [ ] **Strategy names**: Verify strategy names in guide match constants in `internal/model/types.go`.

#### E. Version and Release Readiness
- [ ] **Uncommitted changes**: `git status` should be clean (or only have expected files).
- [ ] **All tests pass**: `go test ./... -count=1 -race` exits 0.
- [ ] **No TODO(release)**: Grep for `TODO.*release` or `FIXME` in Go source — flag any blocking items.
- [ ] **goreleaser dry-run**: If goreleaser is available, `goreleaser check` validates the config.

#### Skill Output Format

The skill should output a structured report:

```
Pre-Release Analysis
====================

README Consistency
  ✓ Command tree matches registered commands
  ✗ Quick Start says "no config needed" but config is required (README.md:17)
  ✗ Flag --project is deprecated, use --project-url (README.md:42)
  ✗ Missing commands in tree: risk bus-factor, status reviews, status my-week

Config Consistency
  ✓ defaultConfigTemplate parses and validates
  ✗ Root .gh-velocity.yml has deprecated field: lifecycle.done.query
  ✓ docs/examples/cli-cli.yml validates

Smoke Test Coverage
  ✗ smoke-test-ext.sh uses removed command: "gh velocity lead-time" (line 66)
  ✗ smoke-test-ext.sh uses removed command: "gh velocity scope" (line 79)
  ✓ All commands have at least one smoke test in smoke-test.sh

Guide Consistency
  ✓ Config field names match schema
  ✓ Strategy names match model constants

Release Readiness
  ✓ Working tree clean
  ✓ All tests pass
  ✓ No blocking TODOs

Summary: 4 issues found, 3 blocking
```

Each finding includes the file and line number for easy navigation.

### 2. `task release` in Taskfile.yaml

A new Taskfile target that runs the full pre-release pipeline:

```yaml
# Taskfile.yaml addition
release:preflight:
  desc: Pre-release validation (run before tagging)
  cmds:
    - task: quality
    - task: e2e:configs
    - task: test:integration
    - echo '✓ All pre-release checks passed — ready to tag'
```

This runs sequentially (per AGENTS.md convention):
1. `quality` (tidy, modernize, test, lint, staticcheck, vulncheck, graphql-injection, smoke)
2. `e2e:configs` (validate example configs against real repos)
3. `test:integration` (extension smoke tests via `smoke-test-ext.sh`)

The skill runs separately via Claude Code (not in Taskfile) because it needs LLM reasoning to compare README prose against code semantics. The Taskfile handles deterministic checks; the skill handles fuzzy/semantic checks.

## Acceptance Criteria

### Skill
- [x] `.claude/skills/pre-release-analysis/SKILL.md` exists with frontmatter
- [x] Skill checks README command tree against registered Cobra commands
- [x] Skill checks README Quick Start commands parse correctly
- [x] Skill checks config fields in README match Go schema structs
- [x] Skill checks `defaultConfigTemplate` round-trips through config validation
- [x] Skill checks example configs validate
- [x] Skill checks smoke-test-ext.sh uses current command names
- [x] Skill checks docs/guide.md field names match schema
- [x] Skill outputs structured report with file:line references
- [x] Skill flags blocking vs non-blocking issues

### Taskfile
- [x] `task release:preflight` exists and runs: quality → e2e:configs → prerelease-analysis
- [x] `task release:preflight` fails fast on first error (sequential cmds)
- [ ] Running `task release:preflight` catches real issues in the current codebase

### Validation
- [x] Run the skill against the current codebase — it should find the known issues (README Quick Start lie, stale smoke-test-ext.sh, missing commands in README tree)
- [ ] Run `task release:preflight` — it should fail on `test:integration` because `smoke-test-ext.sh` is stale
- [ ] After fixing all issues, both the skill and `task release:preflight` should pass clean

## Implementation Notes

### Skill Structure

```
.claude/skills/pre-release-analysis/
└── SKILL.md
```

Follow the pattern from `github-project-workflow/SKILL.md`:
- `disable-model-invocation: true` (user invokes explicitly)
- Allowed tools: Bash, Read, Grep, Glob, Agent
- No Write/Edit — this is read-only analysis, not auto-fix

### Key Files to Inspect

| Check | Files |
|-------|-------|
| Cobra commands | `cmd/root.go` (AddCommand calls) |
| Flag definitions | `cmd/*.go` (Flags().StringVar, etc.) |
| Config schema | `internal/model/types.go`, `internal/config/config.go` |
| Default template | `cmd/config.go` (defaultConfigTemplate) |
| Example configs | `docs/examples/*.yml` |
| Root config | `.gh-velocity.yml` |
| Smoke tests | `scripts/smoke-test.sh`, `scripts/smoke-test-ext.sh` |
| README | `README.md` |
| Guide | `docs/guide.md` |
| Strategy constants | `internal/model/types.go` (StrategyIssue, StrategyPR) |
| Deprecated flags | `cmd/preflight.go` (MarkDeprecated calls) |

## Sources

- Existing skill pattern: `.claude/skills/github-project-workflow/SKILL.md`
- Taskfile conventions: `Taskfile.yaml` (sequential cmds, not parallel deps)
- AGENTS.md conventions: integration tests against built binary, not `go run`
- Known issues found in v0.0.2 readiness plan: `docs/plans/2026-03-12-005-feat-v002-release-readiness-plan.md`
