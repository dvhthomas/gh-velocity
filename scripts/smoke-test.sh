#!/usr/bin/env bash
# Smoke tests — run a small number of real commands against public repos.
# Requires: gh auth (valid GitHub token), built ./gh-velocity binary.
set -euo pipefail

BINARY="./gh-velocity"
PASS=0
FAIL=0
ERRORS=""

pass() { ((PASS++)); echo "  ✓ $1"; }
fail() { ((FAIL++)); ERRORS+="  ✗ $1\n"; echo "  ✗ $1"; }

# Check binary exists
if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'task build' first."
  exit 1
fi

echo "Smoke tests"
echo "==========="

# ── version ────────────────────────────────────────────────────────
echo ""
echo "version"

out=$($BINARY version 2>&1)
[[ "$out" == *"gh-velocity"* ]] && pass "version pretty" || fail "version pretty: $out"

out=$($BINARY version --format json 2>&1)
echo "$out" | jq -e '.version' >/dev/null 2>&1 && pass "version json" || fail "version json: $out"

# ── config ─────────────────────────────────────────────────────────
echo ""
echo "config"

out=$($BINARY config show 2>&1)
[[ "$out" == *"workflow"* ]] && pass "config show" || fail "config show: $out"

out=$($BINARY config validate 2>&1)
[[ "$out" == *"valid"* ]] && pass "config validate" || fail "config validate: $out"

# ── lead-time ──────────────────────────────────────────────────────
echo ""
echo "lead-time (cli/cli#1)"

out=$($BINARY lead-time 1 -R cli/cli 2>&1)
[[ "$out" == *"Lead Time"* ]] && pass "lead-time pretty" || fail "lead-time pretty: $out"

out=$($BINARY lead-time 1 -R cli/cli -f json 2>&1)
echo "$out" | jq -e '.lead_time_seconds' >/dev/null 2>&1 && pass "lead-time json" || fail "lead-time json: $out"

out=$($BINARY lead-time 1 -R cli/cli -f markdown 2>&1)
[[ "$out" == *"|"* ]] && pass "lead-time markdown" || fail "lead-time markdown: $out"

# ── cycle-time ─────────────────────────────────────────────────────
echo ""
echo "cycle-time (cli/cli#1)"

out=$($BINARY cycle-time 1 -R cli/cli 2>&1)
[[ "$out" == *"Cycle Time"* ]] && pass "cycle-time pretty" || fail "cycle-time pretty: $out"

out=$($BINARY cycle-time 1 -R cli/cli -f json 2>&1)
echo "$out" | jq -e '.issue' >/dev/null 2>&1 && pass "cycle-time json" || fail "cycle-time json: $out"

# ── release ────────────────────────────────────────────────────────
echo ""
echo "release (cli/cli v2.65.0)"

out=$($BINARY release v2.65.0 -R cli/cli --since v2.64.0 2>&1)
[[ "$out" == *"Release v2.65.0"* ]] && pass "release pretty" || fail "release pretty: $out"

out=$($BINARY release v2.65.0 -R cli/cli --since v2.64.0 -f json 2>/dev/null)
echo "$out" | jq -e '.tag' >/dev/null 2>&1 && pass "release json" || fail "release json: $out"

out=$($BINARY release v2.65.0 -R cli/cli --since v2.64.0 -f markdown 2>/dev/null)
[[ "$out" == *"## Release v2.65.0"* ]] && pass "release markdown" || fail "release markdown: $out"

# ── scope ──────────────────────────────────────────────────────────
echo ""
echo "scope (cli/cli v2.65.0)"

out=$($BINARY scope v2.65.0 -R cli/cli --since v2.64.0 2>&1)
[[ "$out" == *"Scope: v2.65.0"* ]] && pass "scope pretty" || fail "scope pretty: $out"
[[ "$out" == *"Strategy:"* ]] && pass "scope shows strategies" || fail "scope shows strategies: $out"

out=$($BINARY scope v2.65.0 -R cli/cli --since v2.64.0 -f json 2>/dev/null)
echo "$out" | jq -e '.strategies' >/dev/null 2>&1 && pass "scope json" || fail "scope json: $out"

out=$($BINARY scope v2.65.0 -R cli/cli --since v2.64.0 -f markdown 2>/dev/null)
[[ "$out" == *"## Scope:"* ]] && pass "scope markdown" || fail "scope markdown: $out"

# ── error cases ────────────────────────────────────────────────────
echo ""
echo "error handling"

out=$($BINARY lead-time abc -R cli/cli 2>&1) && fail "bad issue should fail" || pass "bad issue number rejected"

out=$($BINARY --post lead-time 1 -R cli/cli 2>&1) && fail "--post should fail" || pass "--post rejected"

out=$($BINARY lead-time 1 -R cli/cli -f json --post 2>&1 || true)
echo "$out" | jq -e '.error.code' >/dev/null 2>&1 && pass "json error envelope" || fail "json error envelope: $out"

# ── summary ────────────────────────────────────────────────────────
echo ""
echo "==========="
echo "Passed: $PASS  Failed: $FAIL"

if [[ $FAIL -gt 0 ]]; then
  echo ""
  echo "Failures:"
  echo -e "$ERRORS"
  exit 1
fi

echo "All smoke tests passed."
