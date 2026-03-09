---
status: complete
priority: p2
issue_id: 017
tags: [code-review, architecture, cross-repo]
dependencies: []
---

# Define --repo API-Only Fallback Path

## Problem Statement

When `--repo owner/name` is used outside a git checkout, `internal/git/` cannot execute local git commands. The `release` command depends heavily on local git for commit range computation. Without a fallback, `--repo` is broken for the flagship command.

**Raised by:** Spec Flow Analyzer (CRITICAL), Architecture Strategist (mentioned)

## Findings

- Tags are available via REST: `GET /repos/{owner}/{repo}/tags`
- Commit comparison available via: `GET /repos/{owner}/{repo}/compare/{base}...{head}`
- Falling back to API is slower but functional
- Config file discovery also unclear when `--repo` is used without a local clone

## Proposed Solutions

### Option A: Transparent API fallback (Recommended — approved)
- Detect whether local git repo matches `--repo` target
- If not available, use GitHub API for tags and commit comparison
- Skip config file when no local repo; use defaults + flags only
- Warn on stderr: "Using API for git operations (no local checkout)"
- **Effort:** Medium
- **Risk:** Low

## Implementation (Completed)

### Architecture
- Created `internal/gitdata/` package with a `Source` interface abstracting git operations (Tags, CommitsBetween, AllCommits)
- `LocalSource` wraps `git.Runner` for local git CLI operations
- `APISource` wraps `github.Client` for GitHub REST API fallback
- `IsLocalGitAvailable()` detects whether CWD has a `.git` directory

### Changes
- `internal/github/tags.go`: Added `ListTags()` and `CompareCommits()` methods using REST API with pagination
- `internal/gitdata/gitdata.go`: New package with `Source` interface, `LocalSource`, `APISource`, and `IsLocalGitAvailable`
- `cmd/root.go`: Added `HasLocalRepo` field to `Deps`; detects local git; falls back to config defaults when no local repo
- `cmd/release.go`: Uses `gitdata.Source` interface instead of direct `git.Runner`; emits stderr warning when using API fallback
- `cmd/cycletime.go`: Uses `gitdata.Source` interface; warns when commit linking unavailable without local checkout
- `internal/config/config.go`: Exported `Defaults()` for use when no config file is available

### Limitations
- `APISource.AllCommits()` returns an error because the GitHub compare API requires a base ref. Users must use `--since <tag>` when running without a local checkout and no previous tag exists.
- `cycle-time` command cannot link commits to issues without a local checkout (no equivalent single-issue commit search API).

## Acceptance Criteria

- [x] `gh velocity release v1.0.0 --repo owner/name` works outside a git repo
- [x] Transparent fallback to GitHub API when local git unavailable
- [x] Stderr warning when using API fallback
- [x] Config defaults used when no local repo for config file discovery
- [x] Decision documented in plan
