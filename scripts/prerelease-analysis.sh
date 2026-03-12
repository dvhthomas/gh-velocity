#!/usr/bin/env bash
# prerelease-analysis.sh — Tool-agnostic pre-release consistency audit.
#
# Checks README, config, smoke tests, and guide against actual code.
# Returns non-zero if blocking issues are found.
# Compatible with macOS (BSD grep) and Linux (GNU grep).
#
# Usage: bash scripts/prerelease-analysis.sh [--verbose]
set -euo pipefail

VERBOSE="${1:-}"
PASS=0
FAIL=0
WARN=0
FINDINGS=""

pass() { PASS=$((PASS + 1)); FINDINGS="${FINDINGS}\n  ✓ $1"; }
fail() { FAIL=$((FAIL + 1)); FINDINGS="${FINDINGS}\n  ✗ $1"; }
warn() { WARN=$((WARN + 1)); FINDINGS="${FINDINGS}\n  ⚠ $1"; }
section() { FINDINGS="${FINDINGS}\n\n$1"; }

# ── Helpers ───────────────────────────────────────────────────────────

# Extract registered Cobra commands from cmd/ source files.
# Builds the full command tree (e.g., "flow lead-time", "config preflight").
get_registered_commands() {
    local cmds=()

    # version is added with args
    if grep -q 'NewVersionCmd' cmd/root.go; then
        cmds+=("version")
    fi

    # Sub-commands: parse each group file for AddCommand
    # flow.go -> flow lead-time, flow cycle-time, flow throughput
    # quality.go -> quality release
    # risk.go -> risk bus-factor
    # status.go -> status wip, status my-week, status reviews
    # config.go -> config show, config validate, config create, config discover, config preflight

    # get_use_field FILE: extract the Use: "name" field from a Cobra command file
    get_use_field() {
        sed -n 's/.*Use:[[:space:]]*"\([^"]*\)".*/\1/p' "$1" | head -1
    }

    # For each group, find which Go files define the subcommands
    # by looking up the function name, finding the file, and extracting Use:
    add_subcommands() {
        local parent="$1"
        local group_file="$2"
        [ -f "$group_file" ] || return

        # Extract function names from AddCommand calls
        for func in $(sed -n 's/.*cmd\.AddCommand([Nn]ew\([A-Za-z]*\)Cmd().*/\1/p' "$group_file"); do
            # Find which file defines this function
            local target_file
            target_file=$(grep -rl "func [Nn]ew${func}Cmd\b" cmd/*.go 2>/dev/null | head -1)
            if [ -n "$target_file" ]; then
                local use_name
                use_name=$(get_use_field "$target_file")
                if [ -n "$use_name" ]; then
                    # Strip any args after space (e.g., "lead-time [issue]" -> "lead-time")
                    use_name=$(echo "$use_name" | awk '{print $1}')
                    cmds+=("$parent $use_name")
                fi
            fi
        done
    }

    add_subcommands "flow" "cmd/flow.go"
    add_subcommands "quality" "cmd/quality.go"
    add_subcommands "risk" "cmd/risk.go"
    add_subcommands "status" "cmd/status.go"
    add_subcommands "config" "cmd/config.go"

    # report (top-level, leaf)
    if grep -q 'NewReportCmd' cmd/root.go; then
        cmds+=("report")
    fi

    printf '%s\n' "${cmds[@]}" | sort -u
}

# ── A. README ↔ Code Consistency ──────────────────────────────────────

check_readme() {
    section "README Consistency"

    if [ ! -f README.md ]; then
        fail "README.md not found"
        return
    fi

    # A1. Command tree: check each registered command appears in README
    local registered
    registered=$(get_registered_commands)

    local missing=""
    while IFS= read -r cmd; do
        [ -z "$cmd" ] && continue
        # Check if the command (or its parts) appear in README
        # For "flow lead-time", check that "lead-time" appears in a flow context
        local leaf
        leaf=$(echo "$cmd" | awk '{print $NF}')
        if ! grep -q "$leaf" README.md; then
            missing="${missing}${cmd}, "
        fi
    done <<< "$registered"

    if [ -n "$missing" ]; then
        fail "README missing commands: ${missing%, }"
    else
        pass "All registered commands mentioned in README"
    fi

    # A2. Deprecated --project <int> flag without deprecation note
    if grep -n '\-\-project [0-9]' README.md | grep -v 'deprecated\|project-url' >/dev/null 2>&1; then
        local lines
        lines=$(grep -n '\-\-project [0-9]' README.md | grep -v 'deprecated\|project-url' | head -3)
        fail "README references --project <int> without deprecation note: $lines"
    else
        pass "No undocumented deprecated flags"
    fi

    # A3. Quick Start says "no config needed" but config IS required
    if grep -i 'no config' README.md | grep -iE 'needed|required|necessary' >/dev/null 2>&1; then
        fail "README claims no config needed, but config is required for all non-config commands"
    elif grep -i 'without.*config' README.md >/dev/null 2>&1; then
        fail "README implies config is optional (found 'without...config')"
    else
        pass "Quick Start does not falsely claim config is optional"
    fi

    # A4. Old field names in config examples
    if grep -nE 'bug_labels|feature_labels|project\.id[^_]|project: [0-9]' README.md >/dev/null 2>&1; then
        local lines
        lines=$(grep -nE 'bug_labels|feature_labels|project\.id[^_]|project: [0-9]' README.md | head -5)
        fail "README uses deprecated config fields: $lines"
    else
        pass "Config fields in README match current schema"
    fi
}

# ── B. Config Template ↔ Schema ───────────────────────────────────────

check_configs() {
    section "Config Consistency"

    # B1. Example configs should parse as valid YAML
    local example_count=0
    local example_fail=0
    for f in docs/examples/*.yml docs/examples/*.yaml; do
        [ -f "$f" ] || continue
        example_count=$((example_count + 1))
        if [ -x ./gh-velocity ]; then
            if ! ./gh-velocity config validate --config "$f" >/dev/null 2>&1; then
                fail "Example config fails validation: $f"
                example_fail=$((example_fail + 1))
            fi
        fi
    done

    if [ "$example_count" -gt 0 ] && [ "$example_fail" -eq 0 ]; then
        if [ -x ./gh-velocity ]; then
            pass "All $example_count example configs validate"
        else
            warn "Binary not built — skipped example config validation ($example_count configs found)"
        fi
    elif [ "$example_count" -eq 0 ]; then
        warn "No example configs found in docs/examples/"
    fi

    # B2. Root .gh-velocity.yml validation
    if [ -f .gh-velocity.yml ] && [ -x ./gh-velocity ]; then
        local val_output
        if val_output=$(./gh-velocity config validate 2>&1); then
            pass "Root .gh-velocity.yml validates"
        else
            fail "Root .gh-velocity.yml validation errors: $val_output"
        fi
    fi

    # B3. defaultConfigTemplate round-trip
    if [ -x ./gh-velocity ]; then
        local tmpdir
        tmpdir=$(mktemp -d)
        trap "rm -rf $tmpdir" EXIT
        if ./gh-velocity config create --config "$tmpdir/.gh-velocity.yml" >/dev/null 2>&1; then
            if ./gh-velocity config validate --config "$tmpdir/.gh-velocity.yml" >/dev/null 2>&1; then
                pass "defaultConfigTemplate round-trips through create → validate"
            else
                fail "defaultConfigTemplate creates invalid config"
            fi
        else
            warn "config create failed (may need --repo)"
        fi
    fi
}

# ── C. Smoke Test Coverage ────────────────────────────────────────────

check_smoke_tests() {
    section "Smoke Test Coverage"

    # C1. Check smoke-test-ext.sh for stale command names
    if [ -f scripts/smoke-test-ext.sh ]; then
        local stale=""

        # Look for old top-level commands that should now be subcommands
        # "gh velocity lead-time" should be "gh velocity flow lead-time"
        if grep -n 'gh velocity lead-time\|gh velocity lead_time' scripts/smoke-test-ext.sh | grep -v 'flow' >/dev/null 2>&1; then
            local line
            line=$(grep -n 'gh velocity lead-time\|gh velocity lead_time' scripts/smoke-test-ext.sh | grep -v 'flow' | head -1 | cut -d: -f1)
            stale="${stale}lead-time (line $line, now: flow lead-time), "
        fi

        # "gh velocity scope" should be "gh velocity quality release --discover"
        if grep -n 'gh velocity scope' scripts/smoke-test-ext.sh >/dev/null 2>&1; then
            local line
            line=$(grep -n 'gh velocity scope' scripts/smoke-test-ext.sh | head -1 | cut -d: -f1)
            stale="${stale}scope (line $line, now: quality release --discover), "
        fi

        # "gh velocity cycle-time" should be "gh velocity flow cycle-time"
        if grep -n 'gh velocity cycle-time' scripts/smoke-test-ext.sh | grep -v 'flow' >/dev/null 2>&1; then
            stale="${stale}cycle-time (now: flow cycle-time), "
        fi

        # "gh velocity throughput" should be "gh velocity flow throughput"
        if grep -n 'gh velocity throughput' scripts/smoke-test-ext.sh | grep -v 'flow' >/dev/null 2>&1; then
            stale="${stale}throughput (now: flow throughput), "
        fi

        # "gh velocity release" as top-level (deprecated, should be "quality release")
        if grep -n 'gh velocity release' scripts/smoke-test-ext.sh | grep -v 'quality' >/dev/null 2>&1; then
            local line
            line=$(grep -n 'gh velocity release' scripts/smoke-test-ext.sh | grep -v 'quality' | head -1 | cut -d: -f1)
            stale="${stale}release (line $line, now: quality release), "
        fi

        if [ -n "$stale" ]; then
            fail "smoke-test-ext.sh uses stale commands: ${stale%, }"
        else
            pass "smoke-test-ext.sh uses current command names"
        fi
    else
        warn "scripts/smoke-test-ext.sh not found"
    fi

    # C2. Command coverage in smoke-test.sh
    if [ -f scripts/smoke-test.sh ]; then
        local untested=""
        local registered
        registered=$(get_registered_commands)

        while IFS= read -r cmd; do
            [ -z "$cmd" ] && continue
            # Skip group parents (they just print help) and deprecated
            case "$cmd" in
                flow|quality|risk|status|config|release) continue ;;
            esac

            # Extract the leaf command name for searching
            local leaf
            leaf=$(echo "$cmd" | awk '{print $NF}')
            if ! grep -q "$leaf" scripts/smoke-test.sh; then
                untested="${untested}${cmd}, "
            fi
        done <<< "$registered"

        if [ -n "$untested" ]; then
            warn "Commands without smoke test coverage: ${untested%, }"
        else
            pass "All leaf commands have smoke test coverage"
        fi
    else
        warn "scripts/smoke-test.sh not found"
    fi
}

# ── D. Guide ↔ Code Consistency ───────────────────────────────────────

check_guide() {
    section "Guide Consistency"

    if [ ! -f docs/guide.md ]; then
        warn "docs/guide.md not found — skipping guide checks"
        return
    fi

    # D1. Strategy names in guide match model constants
    local guide_strategies
    guide_strategies=$(sed -n 's/.*strategy:[[:space:]]*\([a-z_-]*\).*/\1/p' docs/guide.md | sort -u || true)
    if [ -n "$guide_strategies" ]; then
        local code_strategies
        # Extract strategy constants: StrategyIssue = "issue", StrategyPR = "pr"
        code_strategies=$(sed -n 's/.*Strategy[A-Za-z]*[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' internal/model/types.go 2>/dev/null | sort -u || true)

        local bad=""
        while IFS= read -r s; do
            [ -z "$s" ] && continue
            if ! echo "$code_strategies" | grep -qF "$s"; then
                bad="${bad}${s}, "
            fi
        done <<< "$guide_strategies"

        if [ -n "$bad" ]; then
            fail "Guide references unknown strategies: ${bad%, }"
        else
            pass "Strategy names in guide match model constants"
        fi
    else
        pass "No strategy references found in guide (or matches code)"
    fi

    # D2. Deprecated field names in guide
    if grep -nE 'bug_labels|feature_labels|project\.id[^_]|project: [0-9]' docs/guide.md >/dev/null 2>&1; then
        fail "Guide uses deprecated config field names"
    else
        pass "Guide config field names are current"
    fi
}

# ── E. Release Readiness ──────────────────────────────────────────────

check_release_readiness() {
    section "Release Readiness"

    # E1. Uncommitted changes
    if [ -z "$(git status --porcelain 2>/dev/null)" ]; then
        pass "Working tree clean"
    else
        warn "Working tree has uncommitted changes"
    fi

    # E2. Tests pass
    if go test ./... -count=1 >/dev/null 2>&1; then
        pass "All Go tests pass"
    else
        fail "Go tests failing"
    fi

    # E3. Blocking TODOs
    local blocking
    blocking=$(grep -rn 'TODO.*release\|FIXME' --include='*.go' . 2>/dev/null | grep -v '_test.go' | grep -v vendor/ || true)
    if [ -n "$blocking" ]; then
        local count
        count=$(echo "$blocking" | wc -l | tr -d ' ')
        warn "$count TODO/FIXME items in Go source (review before release)"
    else
        pass "No blocking TODO/FIXME items"
    fi

    # E4. goreleaser config check
    if [ -f .goreleaser.yml ] || [ -f .goreleaser.yaml ]; then
        if command -v goreleaser >/dev/null 2>&1; then
            if goreleaser check >/dev/null 2>&1; then
                pass "goreleaser config valid"
            else
                fail "goreleaser check failed"
            fi
        else
            warn "goreleaser not installed — skipped config check"
        fi
    fi
}

# ── Run all checks ────────────────────────────────────────────────────

echo "Pre-Release Analysis"
echo "===================="

check_readme
check_configs
check_smoke_tests
check_guide
check_release_readiness

# ── Summary ───────────────────────────────────────────────────────────

echo -e "$FINDINGS"
echo ""
echo "Summary: $FAIL blocking, $WARN warnings, $PASS passed"

if [ "$FAIL" -gt 0 ]; then
    echo ""
    echo "Fix $FAIL blocking issue(s) before release."
    exit 1
fi

echo ""
echo "✓ Ready to release"
exit 0
