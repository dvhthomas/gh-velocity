---
title: "feat: my-week cross-repo by default"
type: feat
status: active
date: 2026-03-20
---

# feat: my-week cross-repo by default

## Overview

`gh velocity status my-week` should show ALL of the authenticated user's activity across repos by default, not require a repo context. The `-R` flag becomes an explicit opt-in to limit results to a single repo. This matches the command's purpose: personal 1:1 prep that spans wherever you work.

## Problem Statement

Today `my-week` fails if you're not in a git repo and haven't passed `-R`:

```
not a git repository. Use --repo owner/name
```

This is wrong for a personal activity summary. The search queries already use `author:<login>`, `assignee:<login>`, and `reviewed-by:<login>` — they're inherently user-scoped. The repo requirement is an artifact of the root command's `PersistentPreRunE`, not a real need.

Additionally, when a config file has `scope.query: "repo:owner/name"`, the queries are filtered to that single repo even though the user wants a cross-repo view.

## Proposed Solution

Make repo and config **optional** for `my-week`. The command bypasses `PersistentPreRunE`'s repo resolution and config loading, then handles its own lighter setup.

### Behavior Matrix

| Condition | Scope | Releases | Cycle Time |
|-----------|-------|----------|------------|
| No `-R`, no config | Empty (all repos) | Skipped | Skipped (no strategy) |
| No `-R`, config with `scope.query` | Config scope applies | If config has repo context | Config strategy |
| `-R owner/repo` | `repo:owner/repo` | Fetched | Default (PR) strategy |
| `-R` + config | Merged (`-R` + config scope) | Fetched | Config strategy |

## Technical Approach

### Phase 1: Make my-week bypass root PersistentPreRunE

**`cmd/root.go`** — Add `my-week` to the skip list in `PersistentPreRunE`:

```go
case cmd.Name() == "version":
    return nil
case cmd.Parent() != nil && cmd.Parent().Name() == "config":
    return nil
case cmd.Name() == "my-week":
    return nil // my-week handles its own setup
case cmd.RunE == nil && cmd.Run == nil:
    return nil
```

### Phase 2: Self-contained setup in myweek.go

**`cmd/myweek.go`** — Move dependency setup into `runMyWeek`. The command does its own lighter init:

1. Parse `--since` flag (already done)
2. Try to load config (optional — if file exists, use it; if not, proceed with defaults)
3. Try to resolve repo (optional — if `-R` is set or in a git repo, use it; otherwise leave empty)
4. Build scope: merge config scope + `--scope` flag + repo qualifier (if `-R` was explicit)
5. Create GitHub client with owner/repo if available (empty strings are fine for search-only)
6. Get authenticated user
7. Run search queries with assembled scope (empty scope = cross-repo)
8. Only fetch releases if repo context is available
9. Only compute cycle time if config/strategy is available

Key changes to `runMyWeek`:

```go
func runMyWeek(cmd *cobra.Command, sinceStr string) error {
    ctx := cmd.Context()

    // Parse time range.
    now := nowFunc()()
    since, err := dateutil.Parse(sinceStr, now)
    if err != nil {
        return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
    }

    // Optional config: load if present, nil if not.
    configPath := config.DefaultConfigFile
    if f := cmd.Flag("config"); f != nil && f.Changed {
        configPath = f.Value.String()
    }
    var cfg *config.Config
    if _, statErr := os.Stat(configPath); statErr == nil {
        cfg, err = config.Load(configPath)
        if err != nil {
            return &model.AppError{Code: model.ErrConfigInvalid, Message: err.Error()}
        }
    }

    // Optional repo: -R flag > GH_REPO > git remote > empty.
    repoFlag := ""
    if f := cmd.Flag("repo"); f != nil && f.Changed {
        repoFlag = f.Value.String()
    }
    owner, repo, repoErr := resolveRepo(repoFlag)
    hasRepo := repoErr == nil && owner != "" && repo != ""

    // Build scope.
    var repoScope string
    if cfg != nil {
        repoScope = cfg.Scope.Query
    }
    scopeFlag := ""
    if f := cmd.Flag("scope"); f != nil && f.Changed {
        scopeFlag = f.Value.String()
    }
    repoScope = scope.MergeScope(repoScope, scopeFlag)

    // If -R was explicitly set but no config scope, inject repo: qualifier.
    if hasRepo && repoScope == "" && repoFlag != "" {
        repoScope = fmt.Sprintf("repo:%s/%s", owner, repo)
    }

    // ... rest of the function uses repoScope, hasRepo, cfg as needed
}
```

### Phase 3: Conditional features

**Releases** — only fetch when `hasRepo`:

```go
if hasRepo {
    g.Go(func() error {
        rels, err := client.ListReleases(gCtx, since, now)
        // ...
    })
}
```

**Cycle time** — only compute when config provides a strategy:

```go
var cycleTimeDurations []time.Duration
if cfg != nil {
    strat := buildCycleTimeStrategy(ctx, deps, client)
    cycleTimeDurations = computeMyWeekCycleTime(ctx, strat, result)
}
```

**Exclude users** — only apply when config provides them:

```go
excludeUsers := ""
if cfg != nil {
    excludeUsers = scope.BuildExclusions(cfg.ExcludeUsers)
}
```

### Phase 4: Model and format updates

**`internal/model/types.go`** — Change `Repo string` to `Repo string` (keep as-is, but allow empty string meaning "cross-repo"):

The `Repo` field is already a string. When empty, formatters should display "all repos" or similar instead of a blank.

**`internal/format/myweek.go`** — Handle empty `Repo`:

- Pretty: Show "All repositories" in header when Repo is empty
- Markdown: Same
- JSON: `"repo": null` when empty (or omit)

### Phase 5: GitHub client without owner/repo

**`internal/github/client.go`** — `NewClient` currently takes `owner, repo` — the search methods don't use them (they take a query string). `ListReleases` uses them. When owner/repo are empty, search still works fine. Only `ListReleases` needs guarding (handled in Phase 3).

Verify: `gh.NewClient("", "", delay, opts)` works for search-only usage. If not, adjust to allow empty owner/repo.

### Phase 6: Documentation

**Auto-generated reference page** (`cmd/gendocs/main.go` generates from Cobra definitions):

Update `cmd/myweek.go` Long description and examples:

```go
Long: `Shows what you shipped and what's ahead — designed for 1:1 prep.

Lookback: issues closed, PRs merged, PRs reviewed in the --since period.
Lookahead: open issues assigned to you, open PRs you authored.

By default shows ALL your activity across repositories. Use -R to limit
to a single repo. Uses the authenticated GitHub user (gh auth status).

Works without a config file or repo context — just run it from anywhere.`,

Example: `  # All your activity in the last 7 days
  gh velocity status my-week

  # Limit to a specific repo
  gh velocity status my-week -R owner/repo

  # Last 14 days
  gh velocity status my-week --since 14d

  # Markdown for pasting into a doc
  gh velocity status my-week -r markdown`,
```

**`site/content/guides/recipes.md`** — Update the "Prep for a 1:1 with my-week" recipe to show cross-repo usage and `-R` for single-repo.

## Acceptance Criteria

- [ ] `gh velocity status my-week` works from any directory (no git repo required)
- [ ] `gh velocity status my-week` works without `.gh-velocity.yml`
- [ ] Without `-R`: shows activity across ALL repos for the authenticated user
- [ ] With `-R owner/repo`: shows activity limited to that single repo
- [ ] Releases section only appears when a repo context is available
- [ ] Cycle time only computed when a config with a strategy is available
- [ ] `--scope` flag still works (AND'd with whatever scope exists)
- [ ] Config's `scope.query` still applies when a config file is present
- [ ] `--debug` shows the assembled queries (including empty scope for cross-repo)
- [ ] JSON output handles empty `repo` field gracefully
- [ ] Cobra-generated docs page reflects the new behavior (Long description, examples)
- [ ] `site/content/guides/recipes.md` updated with cross-repo examples
- [ ] All existing `internal/format/myweek_test.go` tests still pass
- [ ] New test: `my-week` with no Deps (nil deps) returns clear error
- [ ] New test: `my-week` with empty scope produces queries without repo qualifier

## Dependencies & Risks

- **GitHub Search API rate limits**: Cross-repo queries may return more results. The existing `g.SetLimit(3)` concurrency throttle helps, but results could be much larger. The search API returns max 1000 results per query — document this limit.
- **`NewClient` with empty owner/repo**: Need to verify `go-gh` doesn't error on empty strings for REST client creation. Search endpoints don't need them, but the client constructor might.
- **Backwards compatibility**: Users who relied on implicit repo scoping via config will see the same behavior (config scope still applies). Only users without config get the new cross-repo behavior.
- **`--scope` / `-R` conflict**: The existing `DetectRepoConflict` warning in root.go won't fire for my-week (since we skip PersistentPreRunE). If both are set, they'll be AND'd — which is correct behavior.

## Sources & References

- `cmd/myweek.go` — current implementation
- `cmd/root.go:177-187` — PersistentPreRunE skip logic
- `cmd/root.go:253-257` — repo resolution
- `cmd/root.go:270-276` — config requirement
- `internal/scope/scope.go` — query builders (already accept empty scope)
- `internal/model/types.go:270-287` — MyWeekResult struct
- `internal/format/myweek.go` — formatters
- `internal/format/myweek_test.go` — formatter tests
- `site/content/guides/recipes.md:171-185` — current my-week recipe
- `cmd/gendocs/main.go` — auto-generated docs
