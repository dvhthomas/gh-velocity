#!/usr/bin/env bash
# Smoke tests — run real commands against public repos and print output.
# stdout: verbose output showing actual stats (useful in CI logs).
# stderr: only on failure (exit 1).
# Requires: gh auth (valid GitHub token), built ./gh-velocity binary.
set -euo pipefail

BINARY="./gh-velocity"
PASS=0
FAIL=0
ERRORS=""

pass() { PASS=$((PASS + 1)); echo "  ✓ $1"; }
fail() { FAIL=$((FAIL + 1)); ERRORS+="  ✗ $1\n"; echo "  ✗ $1" >&2; }

# Print command output indented for readability.
show() { echo "$1" | sed 's/^/    /'; }

# Check binary exists
if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'task build' first." >&2
  exit 1
fi

echo "Smoke tests"
echo "==========="

# ── version ────────────────────────────────────────────────────────
echo ""
echo "version"

out=$($BINARY version 2>&1)
show "$out"
[[ "$out" == *"gh-velocity"* ]] && pass "version pretty" || fail "version pretty"

out=$($BINARY version --format json 2>&1)
show "$out"
echo "$out" | jq -e '.version' >/dev/null 2>&1 && pass "version json" || fail "version json"

# ── config ─────────────────────────────────────────────────────────
echo ""
echo "config"

out=$($BINARY config show 2>&1)
show "$out"
[[ "$out" == *"workflow"* ]] && pass "config show" || fail "config show"

out=$($BINARY config validate 2>&1)
show "$out"
[[ "$out" == *"valid"* ]] && pass "config validate" || fail "config validate"

# ── config discover ───────────────────────────────────────────────
echo ""
echo "config discover (dvhthomas/gh-velocity)"

out=$($BINARY config discover -R dvhthomas/gh-velocity 2>&1)
show "$out"
[[ "$out" == *"PVT_"* ]] && pass "config discover finds project" || fail "config discover finds project"
[[ "$out" == *"Status"* ]] && pass "config discover shows status field" || fail "config discover shows status field"
[[ "$out" == *"Config snippet"* ]] && pass "config discover shows snippet" || fail "config discover shows snippet"

out=$($BINARY config discover -R dvhthomas/gh-velocity -f json 2>&1)
echo "$out" | jq '.[0].id' 2>/dev/null | sed 's/^/    /'
echo "$out" | jq -e '.[0].id' >/dev/null 2>&1 && pass "config discover json" || fail "config discover json"

out=$($BINARY config discover -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"No Projects"* ]] && pass "config discover no projects" || fail "config discover no projects"

# ── lead-time ──────────────────────────────────────────────────────
echo ""
echo "lead-time (cli/cli#1)"

out=$($BINARY lead-time 1 -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"Lead Time"* ]] && pass "lead-time pretty" || fail "lead-time pretty"
[[ "$out" == *"Started"* ]] && pass "lead-time shows started" || fail "lead-time shows started"

out=$($BINARY lead-time 1 -R cli/cli -f json 2>&1)
show "$out"
echo "$out" | jq -e '.lead_time_seconds' >/dev/null 2>&1 && pass "lead-time json" || fail "lead-time json"
echo "$out" | jq -e '.started_at' >/dev/null 2>&1 && pass "lead-time json started_at" || fail "lead-time json started_at"

out=$($BINARY lead-time 1 -R cli/cli -f markdown 2>&1)
show "$out"
[[ "$out" == *"|"* ]] && pass "lead-time markdown" || fail "lead-time markdown"

# ── cycle-time ─────────────────────────────────────────────────────
echo ""
echo "cycle-time (cli/cli#1)"

out=$($BINARY cycle-time 1 -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"Cycle Time"* ]] && pass "cycle-time pretty" || fail "cycle-time pretty"

out=$($BINARY cycle-time 1 -R cli/cli -f json 2>&1)
show "$out"
echo "$out" | jq -e '.issue' >/dev/null 2>&1 && pass "cycle-time json" || fail "cycle-time json"

# ── cycle-time --pr ───────────────────────────────────────────────
echo ""
echo "cycle-time --pr (cli/cli PR#1)"

out=$($BINARY cycle-time --pr 1 -R cli/cli 2>&1)
show "$out"
[[ "$out" == *"Cycle Time"* ]] && pass "cycle-time --pr pretty" || fail "cycle-time --pr pretty"
[[ "$out" == *"Started"* ]] && pass "cycle-time --pr shows started" || fail "cycle-time --pr shows started"

out=$($BINARY cycle-time --pr 1 -R cli/cli -f json 2>&1)
show "$out"
echo "$out" | jq -e '.pr' >/dev/null 2>&1 && pass "cycle-time --pr json" || fail "cycle-time --pr json"
echo "$out" | jq -e '.started_at' >/dev/null 2>&1 && pass "cycle-time --pr json started_at" || fail "cycle-time --pr json started_at"

# ── release ────────────────────────────────────────────────────────
echo ""
echo "release (cli/cli v2.65.0)"

out=$($BINARY release v2.65.0 -R cli/cli --since v2.64.0 2>&1)
show "$out"
[[ "$out" == *"Release v2.65.0"* ]] && pass "release pretty" || fail "release pretty"

out=$($BINARY release v2.65.0 -R cli/cli --since v2.64.0 -f json 2>/dev/null)
echo "$out" | jq . 2>/dev/null | sed 's/^/    /'
echo "$out" | jq -e '.tag' >/dev/null 2>&1 && pass "release json" || fail "release json"

out=$($BINARY release v2.65.0 -R cli/cli --since v2.64.0 -f markdown 2>/dev/null)
show "$out"
[[ "$out" == *"## Release v2.65.0"* ]] && pass "release markdown" || fail "release markdown"

# ── scope ──────────────────────────────────────────────────────────
echo ""
echo "scope (cli/cli v2.65.0)"

out=$($BINARY scope v2.65.0 -R cli/cli --since v2.64.0 2>&1)
show "$out"
[[ "$out" == *"Scope: v2.65.0"* ]] && pass "scope pretty" || fail "scope pretty"
[[ "$out" == *"Strategy:"* ]] && pass "scope shows strategies" || fail "scope shows strategies"

out=$($BINARY scope v2.65.0 -R cli/cli --since v2.64.0 -f json 2>/dev/null)
echo "$out" | jq . 2>/dev/null | sed 's/^/    /'
echo "$out" | jq -e '.strategies' >/dev/null 2>&1 && pass "scope json" || fail "scope json"

out=$($BINARY scope v2.65.0 -R cli/cli --since v2.64.0 -f markdown 2>/dev/null)
show "$out"
[[ "$out" == *"## Scope:"* ]] && pass "scope markdown" || fail "scope markdown"

# ── error cases ────────────────────────────────────────────────────
echo ""
echo "error handling"

out=$($BINARY lead-time abc -R cli/cli 2>&1) && fail "bad issue should fail" || pass "bad issue number rejected"
show "$out"

out=$($BINARY cycle-time 1 --pr 2 -R cli/cli 2>&1) && fail "issue+pr should fail" || pass "issue+pr conflict rejected"
show "$out"

out=$($BINARY --post lead-time 1 -R cli/cli 2>&1) && fail "--post should fail" || pass "--post rejected"
show "$out"

out=$($BINARY lead-time 1 -R cli/cli -f json --post 2>&1 || true)
show "$out"
echo "$out" | jq -e '.error.code' >/dev/null 2>&1 && pass "json error envelope" || fail "json error envelope"

# ── summary ────────────────────────────────────────────────────────
echo ""
echo "==========="
echo "Passed: $PASS  Failed: $FAIL"

if [[ $FAIL -gt 0 ]]; then
  echo "" >&2
  echo "Failures:" >&2
  echo -e "$ERRORS" >&2
  exit 1
fi

echo "All smoke tests passed."
