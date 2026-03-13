# Kantra Refactoring Plan

## Current State

The codebase is a ~25K line Go CLI tool (Cobra-based). The core structural problem is that the `analyze` command has become a **God Object** -- `analyzeCommand` has 38 fields and 57 methods scattered across 4 files. Everything in `cmd/` shares a flat `package cmd` namespace with high coupling.

Key metrics:
- `cmd/analyze.go`: 1,600 lines
- `cmd/analyze-bin.go`: 1,019 lines (containerless mode)
- `cmd/analyze-hybrid.go`: 946 lines (hybrid mode)
- `cmd/cleanup.go`: 157 lines
- `analyzeCommand` struct: 38 fields, 57 methods across 4 files
- ~60% structural duplication between containerless and hybrid execution paths

## Phases

| Phase | Description | Status |
|-------|-------------|--------|
| [Phase 1](phase1-quick-wins.md) | Quick wins -- bug fixes and cleanup | Done |
| [Phase 2](phase2-global-state.md) | Eliminate mutable global state | Done |
| [Phase 3](phase3-god-object.md) | Break up the `analyzeCommand` God Object (3a: adopt `konveyor.Analyzer`, 3b: reorganize into `cmd/analyze/`) | 3a Done, 3b: package move Done, struct decomposition Planned |
| [Phase 4](phase4-developer-experience.md) | Developer experience -- Makefile + test organization | Planned |
| [Phase 5](phase5-environment-interface.md) | `provider.Environment` interface -- unify containerless/hybrid, enable hybrid test runner | In Progress |

Notes:
- Phase 3 subsumes the original "Deduplicate containerless vs hybrid execution" -- adopting `konveyor.Analyzer` from [analyzer-lsp PR #1033](https://github.com/konveyor/analyzer-lsp/pull/1033) eliminates the duplication at the architectural level.
- Phase 3a also covers the `test` command (`pkg/testing/runner.go`) -- replacing its subprocess-based `runLocal()` with a direct in-process `konveyor.Analyzer` call, and extracting shared provider defaults to `pkg/provider/defaults.go`.
- Phase 4 merges the original "Add a Makefile" and "Improve test organization" phases into a single developer experience phase.

## Known Bugs Found During Analysis

1. `pkg/util/util.go:108` -- `CopyFileContents` returns `nil` on `os.Open` error (swallows the error)
2. `cmd/root.go:83-84` -- `log.Fatal` followed by unreachable `os.Exit(1)`
3. `cmd/analyze.go:468` -- Typo "depdendecy" in error message
4. `pkg/container/container.go:148` -- Comment says "WithProxy" but function is `WithPortPublish`
5. `cmd/root.go:2` -- Boilerplate copyright `NAME HERE <EMAIL ADDRESS>`
6. Two incompatible `mergeProviderSpecificConfig` functions with same name but different signatures
