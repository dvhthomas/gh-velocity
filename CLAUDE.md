# gh-velocity Development Guide

A Go-based GitHub CLI extension for velocity and quality metrics.

## Quick Start

```bash
task build       # Build the binary
task test        # Run all tests
task quality     # Lint + staticcheck
task test:coverage  # Tests with coverage report
```

## Architecture

- `main.go` — entry point, calls `cmd.Execute()`
- `cmd/` — one file per Cobra subcommand. Command factory functions: `NewXxxCmd(deps)`.
- `internal/config/` — `.gh-velocity.yml` parsing and validation
- `internal/model/` — shared domain types (pure structs, no API dependency)
- `internal/github/` — GitHub API client (go-gh REST + GraphQL)
- `internal/git/` — local git operations via `exec.CommandContext`
- `internal/metrics/` — pure metric calculations (no API calls)
- `internal/linking/` — commit-to-issue linking heuristics
- `internal/format/` — output formatters (JSON, pretty, markdown)
- `internal/posting/` — GitHub posting (comments, discussions, releases)

## Conventions

- **Go only.** No shell scripts for core logic.
- **Table-driven tests** for all metric calculations and linking heuristics.
- **Narrow interfaces** defined at consumer site, not provider. E.g., `IssueQuerier` in the package that needs it.
- **`context.Context`** propagated from root command through all API and git calls.
- **GraphQL variables only** — never use `fmt.Sprintf` to build GraphQL queries. Always use the `variables` map in `client.Do(query, variables, &result)`. User/config values (`project.id`, `status_field_id`, etc.) must only enter queries as GraphQL variables.
- **Sequential `cmds`** in Taskfile, not parallel `deps` (prevents race conditions).
- **Integration tests** run against the built binary (`task build`), not `go run`.

## Dependencies

- `github.com/cli/go-gh/v2` — auth, REST, GraphQL, tableprinter, repo context
- `github.com/spf13/cobra` — CLI framework
- `gopkg.in/yaml.v3` — config parsing

## Error Handling

Exit codes: 0=success, 1=general, 2=config, 3=auth, 4=not found.
When `--format json`, errors are also JSON: `{"error": "message", "code": N}`.

## Quality

Run `task quality` before every commit. Run `task test` continuously during development.
