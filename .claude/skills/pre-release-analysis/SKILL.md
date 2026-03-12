---
name: pre-release-analysis
description: Detects documentation drift, config mismatches, stale tests, and README inaccuracies before a release. Runs deterministic checks via scripts/prerelease-analysis.sh and performs semantic analysis that requires LLM reasoning.
disable-model-invocation: true
allowed-tools:
  - Bash
  - Read
  - Grep
  - Glob
  - Agent
---

# Pre-Release Analysis

Comprehensive consistency audit before tagging a release. Combines deterministic shell checks with semantic LLM analysis.

## When to Use

Run before any release tag. Invoked manually via `/pre-release-analysis` or as part of the release workflow.

## Step 1: Run Deterministic Checks

Execute the tool-agnostic analysis script:

```bash
bash scripts/prerelease-analysis.sh
```

This checks:
- README command tree vs registered Cobra commands
- Deprecated flags documented without deprecation notes
- Quick Start claims vs actual config requirements
- Config template round-trip validity
- Example config validation
- Smoke test currency (stale command names)
- Smoke test coverage (untested commands)
- Guide strategy names vs model constants
- Guide config field names vs schema
- Working tree cleanliness
- Go test suite
- Blocking TODOs/FIXMEs
- goreleaser config

## Step 2: Semantic Analysis (LLM-powered)

The shell script catches structural mismatches. These checks catch semantic drift that requires reading comprehension:

### 2a. README Accuracy

Read `README.md` and cross-reference against actual behavior:

1. **Installation instructions** — do they work with the current binary name and `gh extension install` flow?
2. **Example output** — does sample output in README match what the tool actually produces? Run a command and compare.
3. **Feature claims** — does README describe features that don't exist yet, or omit features that do exist?
4. **Config examples** — do inline YAML snippets use current field names and valid values?

### 2b. Guide Completeness

Read `docs/guide.md` and check:

1. **All commands documented** — every leaf command in the tree should have at least a mention
2. **Config field coverage** — every config field in the Go structs should be explained
3. **Workflow accuracy** — does the described workflow match how the tool actually works?

### 2c. CHANGELOG / Release Notes Prep

Check what changed since the last release tag:

```bash
# Changes since last tag
LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
if [ -n "$LAST_TAG" ]; then
    git log --oneline "$LAST_TAG"..HEAD
fi
```

Flag any changes that should be called out in release notes (breaking changes, new commands, deprecations).

## Output

Present a unified report combining shell script output and semantic findings. Group by severity:

1. **Blocking** — must fix before release (fail items from script + semantic errors)
2. **Warnings** — should review (warn items from script + semantic concerns)
3. **Passed** — confirmed correct

Include file paths and line numbers for all findings.
