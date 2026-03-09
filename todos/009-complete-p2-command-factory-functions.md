---
status: complete
priority: p2
issue_id: 009
tags: [code-review, architecture, testing, cobra]
dependencies: []
---

# Use Command Factory Functions Instead of init() Registration

## Problem Statement

Cobra's default pattern uses `init()` + `rootCmd.AddCommand()` which encourages package-level globals, making dependency injection awkward and commands hard to test in isolation.

**Raised by:** Architecture Strategist (MEDIUM), Pattern Recognition (MEDIUM)

## Findings

- go-calcmark's institutional learnings already warn "Never call config in init()"
- Factory functions enable proper dependency injection and testable commands
- Each command file should export `NewXxxCmd(deps) *cobra.Command`
- `root.go` wires dependencies in `PersistentPreRunE` and passes them down

## Proposed Solutions

### Option A: Factory functions with dependency struct (Recommended)
```go
// cmd/release.go
func NewReleaseCmd(deps *Dependencies) *cobra.Command { ... }

// cmd/root.go
func NewRootCmd() *cobra.Command {
    deps := &Dependencies{}
    root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
        deps.Config = config.Load(...)
        deps.Client = github.NewClient(...)
        return nil
    }
    root.AddCommand(NewReleaseCmd(deps))
}
```
- **Effort:** Small (pattern choice at Phase 1, not a refactor)
- **Risk:** Low

## Acceptance Criteria

- [x] No `init()` functions registering commands
- [x] Each command created via `NewXxxCmd(deps)` factory
- [x] Commands testable by passing mock dependencies

## Resolution

All commands already used factory functions from the start. The codebase was built with the correct pattern:

- `NewRootCmd(version, buildTime)` - exported root factory that wires all subcommands
- `NewReleaseCmd()` - release command factory
- `NewLeadTimeCmd()` - lead-time command factory
- `NewCycleTimeCmd()` - cycle-time command factory
- `NewVersionCmd(version, buildTime)` - version command factory
- `NewConfigCmd()` - config command factory with sub-factories for show/validate
- `Deps` struct injected via context in `PersistentPreRunE`
- No `init()` functions anywhere in the cmd package
- `Execute()` wraps `NewRootCmd()` for use by `main.go`

Minor change made: exported `newRootCmd` to `NewRootCmd` for consistency and external testability.
