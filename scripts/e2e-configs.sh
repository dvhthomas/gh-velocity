#!/usr/bin/env bash
# E2E tests — run each example config against its target repo and validate JSON output.
# Requires: gh auth (valid GitHub token), built ./gh-velocity binary.
set -euo pipefail

# Resolve repo root so this script works from worktrees and subdirectories.
REPO_ROOT="$(git rev-parse --show-toplevel)"
BINARY="${REPO_ROOT}/gh-velocity"
PASS=0
FAIL=0
ERRORS=""

pass() { PASS=$((PASS + 1)); echo "  ✓ $1"; }
fail() { FAIL=$((FAIL + 1)); ERRORS+="  ✗ $1\n"; echo "  ✗ $1" >&2; }

# Check binary exists
if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'task build' first." >&2
  exit 1
fi

echo "E2E config tests"
echo "================"

# Each entry: config_file repo tag since_tag
configs=(
  "${REPO_ROOT}/docs/examples/cli-cli.yml|cli/cli|v2.87.3|v2.87.2"
  "${REPO_ROOT}/docs/examples/kubernetes-kubernetes.yml|kubernetes/kubernetes|v1.35.2|v1.34.5"
  "${REPO_ROOT}/docs/examples/hashicorp-terraform.yml|hashicorp/terraform|v1.14.6|v1.14.5"
  "${REPO_ROOT}/docs/examples/astral-sh-uv.yml|astral-sh/uv|0.10.9|0.10.8"
  "${REPO_ROOT}/docs/examples/facebook-react.yml|facebook/react|v19.2.4|v19.1.5"
)

for entry in "${configs[@]}"; do
  IFS='|' read -r config repo tag since <<< "$entry"
  name=$(basename "$config" .yml)

  echo ""
  echo "$name ($repo $tag)"

  # Run quality release command with the example config
  out=$($BINARY quality release "$tag" --since "$since" -R "$repo" --config "$config" -r json 2>/dev/null) || {
    fail "$name: command failed"
    continue
  }

  # Validate JSON is parseable
  if ! echo "$out" | jq . >/dev/null 2>&1; then
    fail "$name: invalid JSON output"
    continue
  fi

  # Validate tag field matches
  got_tag=$(echo "$out" | jq -r '.tag' 2>/dev/null)
  if [[ "$got_tag" == "$tag" ]]; then
    pass "$name: tag matches"
  else
    fail "$name: expected tag $tag, got $got_tag"
  fi

  # Validate we got some data (composition.total_issues or issues array)
  total=$(echo "$out" | jq -r '.composition.total_issues // 0' 2>/dev/null)
  issues_len=$(echo "$out" | jq -r '.issues | length // 0' 2>/dev/null)
  if [[ "$total" -gt 0 ]] || [[ "$issues_len" -gt 0 ]]; then
    pass "$name: has issue data (total=$total, issues=$issues_len)"
  else
    # Some releases may legitimately have 0 issues; just warn
    echo "  ⚠ $name: no issues found (may be expected for this release pair)"
  fi

  # Validate composition metrics exist
  if echo "$out" | jq -e '.composition.total_issues >= 0' >/dev/null 2>&1; then
    pass "$name: has composition metrics"
  else
    fail "$name: missing composition metrics"
  fi

  # Validate aggregates exist
  if echo "$out" | jq -e '.aggregates.lead_time' >/dev/null 2>&1; then
    pass "$name: has aggregates"
  else
    fail "$name: missing aggregates"
  fi
done

# ── summary ────────────────────────────────────────────────────────
echo ""
echo "================"
echo "Passed: $PASS  Failed: $FAIL"

if [[ $FAIL -gt 0 ]]; then
  echo "" >&2
  echo "Failures:" >&2
  echo -e "$ERRORS" >&2
  exit 1
fi

echo "All E2E config tests passed."
