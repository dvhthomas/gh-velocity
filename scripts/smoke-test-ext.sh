#!/usr/bin/env bash
# Smoke tests for the installed gh velocity extension.
# Requires: gh auth, `task install` run first.
set -euo pipefail

# Resolve repo root so this script works from worktrees and subdirectories.
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

PASS=0
FAIL=0
ERRORS=""

pass() { ((PASS++)); echo "  ✓ $1"; }
fail() { ((FAIL++)); ERRORS+="  ✗ $1\n"; echo "  ✗ $1"; }

# Verify extension is installed
if ! gh velocity version >/dev/null 2>&1; then
  echo "ERROR: gh velocity not installed. Run 'task install' first."
  exit 1
fi

# Config files for external repos (config is required for all non-config commands).
CLI_CONFIG="${REPO_ROOT}/docs/examples/cli-cli.yml"

echo "Extension smoke tests (gh velocity)"
echo "===================================="

# ── version ────────────────────────────────────────────────────────
echo ""
echo "version"

out=$(gh velocity version 2>&1)
[[ "$out" == *"gh-velocity"* ]] && pass "version" || fail "version: $out"

out=$(gh velocity version -f json 2>&1)
echo "$out" | jq -e '.version' >/dev/null 2>&1 && pass "version json" || fail "version json: $out"

# ── config create ─────────────────────────────────────────────────
echo ""
echo "config"

# Ensure no leftover config
rm -f .gh-velocity.yml

out=$(gh velocity config create 2>&1)
[[ "$out" == *"Created"* ]] && pass "config create" || fail "config create: $out"
[[ -f .gh-velocity.yml ]] && pass "config file exists" || fail "config file not created"

# Should refuse to overwrite
out=$(gh velocity config create 2>&1) && fail "config create should refuse overwrite" || pass "config create refuses overwrite"

out=$(gh velocity config show 2>&1)
[[ "$out" == *"workflow"* ]] && pass "config show" || fail "config show: $out"

out=$(gh velocity config validate 2>&1)
[[ "$out" == *"valid"* ]] && pass "config validate" || fail "config validate: $out"

# Clean up config created by config tests (we use --config for remaining tests)
rm -f .gh-velocity.yml

# ── flow lead-time ─────────────────────────────────────────────────
echo ""
echo "flow lead-time (cli/cli#1)"

out=$(gh velocity flow lead-time 1 -R cli/cli --config "$CLI_CONFIG" 2>&1)
[[ "$out" == *"Lead Time"* ]] && pass "flow lead-time pretty" || fail "flow lead-time pretty: $out"

out=$(gh velocity flow lead-time 1 -R cli/cli --config "$CLI_CONFIG" -f json 2>&1)
echo "$out" | jq -e '.lead_time_seconds' >/dev/null 2>&1 && pass "flow lead-time json" || fail "flow lead-time json: $out"

out=$(gh velocity flow lead-time 1 -R cli/cli --config "$CLI_CONFIG" -f markdown 2>&1)
[[ "$out" == *"|"* ]] && pass "flow lead-time markdown" || fail "flow lead-time markdown: $out"

# ── quality release --discover ────────────────────────────────────
echo ""
echo "quality release --discover (cli/cli v2.65.0)"

out=$(gh velocity quality release v2.65.0 -R cli/cli --config "$CLI_CONFIG" --since v2.64.0 --discover 2>&1)
[[ "$out" == *"Strategy:"* ]] && pass "quality release --discover strategies" || fail "quality release --discover strategies: $out"

out=$(gh velocity quality release v2.65.0 -R cli/cli --config "$CLI_CONFIG" --since v2.64.0 --discover -f json 2>/dev/null)
echo "$out" | jq -e '.strategies' >/dev/null 2>&1 && pass "quality release --discover json" || fail "quality release --discover json: $out"

# ── quality release ───────────────────────────────────────────────
echo ""
echo "quality release (cli/cli v2.65.0)"

out=$(gh velocity quality release v2.65.0 -R cli/cli --config "$CLI_CONFIG" --since v2.64.0 2>&1)
[[ "$out" == *"Release v2.65.0"* ]] && pass "quality release pretty" || fail "quality release pretty: $out"
[[ "$out" == *"Aggregates"* ]] && pass "quality release aggregates" || fail "quality release aggregates: $out"
[[ "$out" == *"P90"* ]] && pass "quality release has P90" || fail "quality release missing P90: $out"

out=$(gh velocity quality release v2.65.0 -R cli/cli --config "$CLI_CONFIG" --since v2.64.0 -f json 2>/dev/null)
echo "$out" | jq -e '.aggregates.lead_time.p90_seconds' >/dev/null 2>&1 && pass "quality release json p90" || fail "quality release json p90: $out"
echo "$out" | jq -e '.aggregates.lead_time.outlier_cutoff_seconds' >/dev/null 2>&1 && pass "quality release json outlier cutoff" || fail "quality release json outlier cutoff: $out"

out=$(gh velocity quality release v2.65.0 -R cli/cli --config "$CLI_CONFIG" --since v2.64.0 -f markdown 2>/dev/null)
[[ "$out" == *"## Release v2.65.0"* ]] && pass "quality release markdown" || fail "quality release markdown: $out"
[[ "$out" == *"Outliers"* ]] && pass "quality release markdown outliers" || fail "quality release markdown outliers: $out"

# ── my-week ───────────────────────────────────────────────────────
echo ""
echo "my-week (dvhthomas/gh-velocity)"

out=$(gh velocity status my-week -R dvhthomas/gh-velocity --config "${REPO_ROOT}/docs/examples/dvhthomas-gh-velocity.yml" --since 30d 2>&1)
[[ "$out" == *"My Week"* ]] && pass "my-week pretty" || fail "my-week pretty: $out"
[[ "$out" == *"What I shipped"* ]] && pass "my-week lookback" || fail "my-week missing lookback: $out"
[[ "$out" == *"What's ahead"* ]] && pass "my-week lookahead" || fail "my-week missing lookahead: $out"

out=$(gh velocity status my-week -R dvhthomas/gh-velocity --config "${REPO_ROOT}/docs/examples/dvhthomas-gh-velocity.yml" --since 30d -f json 2>&1)
echo "$out" | jq -e '.login' >/dev/null 2>&1 && pass "my-week json has login" || fail "my-week json: $out"
echo "$out" | jq -e '.summary.issues_closed >= 0' >/dev/null 2>&1 && pass "my-week json summary" || fail "my-week json summary: $out"
echo "$out" | jq -e '.lookback' >/dev/null 2>&1 && pass "my-week json lookback" || fail "my-week json lookback: $out"
echo "$out" | jq -e '.ahead' >/dev/null 2>&1 && pass "my-week json ahead" || fail "my-week json ahead: $out"

out=$(gh velocity status my-week -R dvhthomas/gh-velocity --config "${REPO_ROOT}/docs/examples/dvhthomas-gh-velocity.yml" --since 30d -f markdown 2>&1)
[[ "$out" == *"## My Week"* ]] && pass "my-week markdown" || fail "my-week markdown: $out"
[[ "$out" == *"What I shipped"* ]] && pass "my-week markdown lookback" || fail "my-week markdown lookback: $out"
[[ "$out" == *"What's ahead"* ]] && pass "my-week markdown lookahead" || fail "my-week markdown lookahead: $out"

# ── error handling ─────────────────────────────────────────────────
echo ""
echo "error handling"

out=$(gh velocity flow lead-time abc -R cli/cli --config "$CLI_CONFIG" 2>&1) && fail "bad issue should fail" || pass "bad issue rejected"
out=$(gh velocity --post flow lead-time 1 -R cli/cli --config "$CLI_CONFIG" 2>&1) && fail "--post should fail" || pass "--post rejected"

# ── summary ────────────────────────────────────────────────────────
echo ""
echo "===================================="
echo "Passed: $PASS  Failed: $FAIL"

if [[ $FAIL -gt 0 ]]; then
  echo ""
  echo "Failures:"
  echo -e "$ERRORS"
  exit 1
fi

echo "All extension smoke tests passed."
