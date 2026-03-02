# Phase 3: Break Up the `analyzeCommand` God Object

## Prerequisites

- **Phase 2 completed** -- `util.SourceMountPath` is no longer mutated at runtime
- **analyzer-lsp PR #1033 merged** -- The `konveyor` package provides `konveyor.Analyzer` with `ParseRules()`, `ProviderStart()`, `Run()`, `GetDependencies()`, and `Stop()`

## Current State

The `analyzeCommand` is a God Object with:
- **38 fields** on the struct + **11 fields** on the embedded `AnalyzeCommandContext` = 49 total fields
- **57 methods** scattered across 5 files (`analyze.go`, `analyze-bin.go`, `analyze-hybrid.go`, `command-context.go`, `cleanup.go`)
- **~60% structural duplication** between `RunAnalysisContainerless()` (319 lines) and `RunAnalysisHybridInProcess()` (528 lines)
- **Two incompatible `mergeProviderSpecificConfig`** functions with the same name

---

## Full Inventory

### Fields (49 total)

**`analyzeCommand` direct fields** (38 fields, `analyze.go:51-89`):

| Category | Fields |
|----------|--------|
| List/info flags (4) | `listSources`, `listTargets`, `listProviders`, `listLanguages` |
| Analysis behavior (7) | `skipStaticReport`, `analyzeKnownLibraries`, `jsonOutput`, `overwrite`, `bulk`, `enableDefaultRulesets`, `noDepRules` |
| Input/output (3) | `input`, `output`, `mode` |
| Rule config (3) | `rules`, `tempRuleDir`, `labelSelector` |
| Source/target labels (2) | `sources`, `targets` |
| Provider config (5) | `provider`, `runLocal`, `disableMavenSearch`, `overrideProviderSettings`, `mavenSettingsFile` |
| Proxy config (3) | `httpProxy`, `httpsProxy`, `noProxy` |
| Runtime/debug (5) | `logLevel`, `cleanup`, `noProgress`, `jaegerEndpoint`, `contextLines` |
| Dependency (2) | `depFolders`, `incidentSelector` |
| Profile (2) | `profileDir`, `profilePath` |

**`AnalyzeCommandContext` embedded fields** (11 fields, `command-context.go:24-43`):

| Category | Fields |
|----------|--------|
| Provider runtime state (2) | `providersMap`, `providerContainerNames` |
| Container resources (3) | `networkName`, `volumeName`, `mavenCacheVolumeName` |
| Temp storage (2) | `tempDirs`, `kantraDir` |
| Input state (2) | `isFileInput`, `needsBuiltin` |
| Logging (1) | `log` |
| Containerless state (1) | `reqMap` |

### Methods (57 total across 5 files)

**`analyze.go` -- 27 methods:**

| Method | Lines | Category |
|--------|-------|----------|
| `NewAnalyzeCmd` | 92-333 | Command setup |
| `Validate` | 336-493 | Validation |
| `CheckOverwriteOutput` | 495-524 | Validation |
| `ValidateAndLoadProfile` | 526-562 | Validation/Profile |
| `validateProviders` | 564-579 | Validation |
| `validateRulesPath` | 581-604 | Validation |
| `needDefaultRules` | 606-618 | Rules |
| `ListAllProviders` | 620-639 | Listing |
| `parseLabelLines` (free func) | 643-654 | Labels |
| `ListLabels` | 656-658 | Labels |
| `fetchLabels` | 660-724 | Labels/Container |
| `readRuleFilesForLabels` | 726-740 | Labels |
| `getDepsFolders` | 742-755 | Volume config |
| `getConfigVolumes` | 757-860 | Provider config |
| `getRulesVolumes` | 862-933 | Rules/Volume |
| `extractDefaultRulesets` | 950-994 | Rules/Container |
| `RunProvidersHostNetwork` | 1020-1092 | Provider execution |
| `CreateJSONOutput` | 1125-1184 | Output |
| `moveResults` | 1186-1219 | Output |
| `inputShortName` | 1221-1223 | Utility |
| `getLabelSelector` | 1225-1271 | Labels |
| `writeProvConfig` | 1273-1286 | Provider config |
| `getProviderOptions` | 1288-1323 | Provider config |
| `mergeProviderConfig` | 1325-1363 | Provider config |
| `mergeProviderSpecificConfig` | 1365-1394 | Provider config |
| `getProviderLogs` | 1396-1422 | Provider runtime |
| `detectJavaProviderFallback` | 1424-1452 | Provider detection |
| `setupProgressReporter` (free func) | 1457-1556 | Progress |
| `listLanguages` (free func) | 1558-1570 | Listing |
| `createProfileSettings` | 1572-1582 | Profile |
| `applyProfileSettings` | 1584-1599 | Profile |

**`analyze-bin.go` -- 19 methods:**

| Method | Lines | Category |
|--------|-------|----------|
| `ConsoleHook` (struct + 2 methods) | 40-59 | Progress/Logging |
| `renderProgressBar` (free func) | 62-79 | Progress |
| `RunAnalysisContainerless` | 81-400 | Execution (containerless) |
| `ValidateContainerless` | 402-451 | Validation |
| `listLabelsContainerless` | 453-455 | Labels |
| `fetchLabelsContainerless` | 457-482 | Labels |
| `walkRuleFilesForLabelsContainerless` | 484-504 | Labels |
| `setBinMapContainerless` | 506-520 | Provider config |
| `makeBuiltinProviderConfig` | 522-539 | Provider config |
| `makeJavaProviderConfig` | 541-576 | Provider config |
| `createProviderConfigsContainerless` | 578-611 | Provider config |
| `setConfigsContainerless` | 613-659 | Provider config |
| `setBuiltinProvider` | 661-682 | Provider setup |
| `setJavaProvider` | 684-699 | Provider setup |
| `setupJavaProvider` | 701-740 | Provider setup |
| `setupBuiltinProvider` | 742-785 | Provider setup |
| `setInternalProviders` | 787-811 | Provider setup |
| `startProvidersContainerless` | 813-845 | Provider execution |
| `DependencyOutputContainerless` | 847-906 | Output |
| `buildStaticReportFile` | 908-962 | Output/Report |
| `buildStaticReportOutput` | 964-974 | Output/Report |
| `GenerateStaticReport` | 976-1018 | Output/Report |

**`analyze-hybrid.go` -- 8 methods:**

| Method | Lines | Category |
|--------|-------|----------|
| `validateProviderConfig` | 45-77 | Validation |
| `loadOverrideProviderSettings` | 81-100 | Provider config |
| `mergeProviderSpecificConfig` (free func) | 104-119 | Provider config (DUPLICATE NAME) |
| `applyProviderOverrides` (free func) | 123-160 | Provider config |
| `waitForProvider` (free func) | 164-211 | Provider runtime |
| `setupNetworkProvider` | 216-300 | Provider setup |
| `runParallelStartupTasks` | 304-364 | Execution orchestration |
| `setupBuiltinProviderHybrid` | 368-403 | Provider setup |
| `RunAnalysisHybridInProcess` | 417-945 | Execution (hybrid) |

**`command-context.go` -- 9 methods:**

| Method | Lines | Category |
|--------|-------|----------|
| `setProviders` | 45-70 | Provider detection |
| `setProviderInitInfo` | 72-113 | Provider config |
| `handleDir` | 115-133 | Rules/File ops |
| `createTempRuleSet` | 135-157 | Rules |
| `setupCommandOutput` | 163-176 | Logging utility |
| `logCommandOutput` | 180-187 | Logging utility |
| `createContainerNetwork` | 189-208 | Container ops |
| `createContainerVolume` | 211-274 | Container ops |
| `createMavenCacheVolume` | 295-369 | Container ops |

**`cleanup.go` -- 6 methods:**

| Method | Lines | Category |
|--------|-------|----------|
| `CleanAnalysisResources` | 9-34 | Cleanup |
| `RmNetwork` | 36-48 | Cleanup |
| `RmVolumes` | 50-78 | Cleanup |
| `RmProviderContainers` | 80-107 | Cleanup |
| `StopProvider` | 109-137 | Cleanup |
| `cleanlsDirs` | 139-157 | Cleanup |

---

## Execution Flow (Call Graph)

```
PreRunE:
  ├── GetKantraDir()
  ├── ValidateAndLoadProfile()
  ├── applyProfileSettings()
  └── Validate()
        ├── fetchLabels / fetchLabelsContainerless()
        ├── validateRulesPath()
        └── CheckOverwriteOutput()

RunE:
  ├── ListAllProviders()              [early return]
  ├── listLabelsContainerless()       [early return]
  ├── ListLabels()                    [early return]
  ├── recognizer.Analyze()            [language detection]
  ├── listLanguages()                 [early return]
  ├── setProviders()
  ├── validateProviders()
  ├── detectJavaProviderFallback()
  ├── setProviderInitInfo()
  │
  ├── [CONTAINERLESS PATH]
  │   └── RunAnalysisContainerless()
  │       ├── ValidateContainerless()
  │       ├── setBinMapContainerless()
  │       ├── setupProgressReporter()
  │       ├── setupJavaProvider() → makeJavaProviderConfig() + setJavaProvider()
  │       ├── setupBuiltinProvider() → makeBuiltinProviderConfig() + setBuiltinProvider()
  │       ├── engine.CreateRuleEngine()         ← DUPLICATED
  │       ├── parser.LoadRules() loop           ← DUPLICATED
  │       ├── DependencyOutputContainerless()   ← DUPLICATED
  │       ├── eng.RunRulesWithOptions()         ← DUPLICATED
  │       ├── CreateJSONOutput()                ← SHARED
  │       └── GenerateStaticReport()            ← SHARED
  │
  └── [HYBRID PATH]
      └── RunAnalysisHybridInProcess()
          ├── loadOverrideProviderSettings()
          ├── runParallelStartupTasks()
          │   ├── validateProviderConfig()
          │   ├── createContainerVolume()
          │   └── extractDefaultRulesets()
          ├── RunProvidersHostNetwork()
          ├── waitForProvider()
          ├── setupNetworkProvider()
          ├── setupBuiltinProviderHybrid()
          ├── engine.CreateRuleEngine()         ← DUPLICATED
          ├── parser.LoadRules() loop           ← DUPLICATED
          ├── DependencyOutputContainerless()   ← DUPLICATED
          ├── eng.RunRulesWithOptions()         ← DUPLICATED
          ├── getProviderLogs()
          ├── CreateJSONOutput()                ← SHARED
          └── GenerateStaticReport()            ← SHARED
```

---

## Phase 3a: Adopt `konveyor.Analyzer` to Eliminate Duplication

### Key insight from analyzer-lsp PR #1033

The new `konveyor` package (`github.com/konveyor/analyzer-lsp/konveyor`) provides:

```go
type Analyzer interface {
    ProviderStart() error
    ParseRules(...string) (Rules, error)
    Run(options ...EngineOption) []v1.RuleSet
    GetDependencies(outputFilePath string, tree bool) error
    GetProviders(...Filter) []Provider
    GetProviderForLanguage(language string) (Provider, bool)
    Stop() error
    // + Rules and Engine interfaces
}

// Created via functional options:
analyzer, err := konveyor.NewAnalyzer(
    konveyor.WithProviderConfigFilePath(settingsPath),
    konveyor.WithRuleFilepaths(rules),
    konveyor.WithLabelSelector(selector),
    konveyor.WithContextLinesLimit(contextLines),
    konveyor.WithIncidentSelector(incidentSelector),
    konveyor.WithLogger(log),
    konveyor.WithContext(ctx),
    konveyor.WithProgress(progress),
)
```

This absorbs the **entire shared execution pipeline**.

### New execution flow after adoption

```
RunE:
  ├── Detect providers + mode (unchanged)
  │
  ├── [CONTAINERLESS]
  │   ├── ValidateContainerless()
  │   ├── Generate provider_settings.json (local paths)
  │   ├── analyzer := konveyor.NewAnalyzer(options...)
  │   ├── analyzer.ParseRules()
  │   ├── analyzer.ProviderStart()
  │   ├── rulesets := analyzer.Run()
  │   ├── analyzer.GetDependencies(depFile, false)
  │   ├── analyzer.Stop()
  │   ├── Write output.yaml from rulesets
  │   ├── CreateJSONOutput()
  │   └── GenerateStaticReport()
  │
  └── [HYBRID]
      ├── Start provider containers (kantra container ops -- unchanged)
      ├── Wait for provider health checks
      ├── Generate provider_settings.json (network addresses)
      ├── analyzer := konveyor.NewAnalyzer(same pattern)
      ├── analyzer.ParseRules()
      ├── analyzer.ProviderStart()
      ├── rulesets := analyzer.Run()
      ├── analyzer.GetDependencies(depFile, false)
      ├── analyzer.Stop()
      ├── Write output.yaml from rulesets
      ├── CreateJSONOutput()
      └── GenerateStaticReport()
```

Both modes converge at `konveyor.NewAnalyzer()` with different `provider_settings.json` files.

### Implementation steps

**Step 1: Update `go.mod`**

Update `github.com/konveyor/analyzer-lsp` to the version containing the `konveyor` package.

**Step 2: Extract shared provider defaults**

Provider configuration is duplicated in three places:

| Location | Lines | Mode |
|----------|-------|------|
| `pkg/testing/runner.go:46-126` | 80 | `defaultProviderConfig` (container paths) |
| `cmd/analyze-bin.go:522-576` | 55 | `makeJavaProviderConfig` + `makeBuiltinProviderConfig` (local paths) |
| `cmd/analyze-hybrid.go:222-260` | 40 | `setupNetworkProvider` switch (container paths) |

There is an existing TODO at `pkg/testing/runner.go:44` acknowledging this:
```go
// TODO (pgaikwad): we need to move the default config to a common place
// to be shared between kantra analyze command and this
```

Create `pkg/provider/defaults.go` with a shared function:

```go
package provider

type ExecutionMode int

const (
    ModeLocal     ExecutionMode = iota  // containerless -- local binary paths
    ModeContainer                        // in-container -- /usr/local/bin paths
    ModeNetwork                          // hybrid -- localhost:PORT addresses
)

// DefaultProviderConfig returns provider configs with paths resolved
// for the given execution mode. This is the single source of truth for
// provider configuration across analyze, test, and hybrid commands.
func DefaultProviderConfig(mode ExecutionMode) []provider.Config { ... }
```

This replaces `defaultProviderConfig` in `pkg/testing/runner.go`, `makeJavaProviderConfig()`/`makeBuiltinProviderConfig()` in `analyze-bin.go`, and the provider-specific switch in `analyze-hybrid.go:222-260`.

**Step 3: Rewrite `RunAnalysisContainerless()` (`analyze-bin.go`)**

Keep:
- `ValidateContainerless()`
- Binary detection, tracing setup, progress mode
- Provider config generation (now using `DefaultProviderConfig(ModeLocal)`)

Replace with `konveyor.NewAnalyzer()` pipeline:
- All code from provider creation through rule execution and dependency analysis

Keep after analyzer returns:
- `CreateJSONOutput()`
- `GenerateStaticReport()`

**Step 4: Rewrite `RunAnalysisHybridInProcess()` (`analyze-hybrid.go`)**

Keep:
- Container orchestration (`runParallelStartupTasks`, `RunProvidersHostNetwork`, `waitForProvider`)
- Provider settings generation for network mode (now using `DefaultProviderConfig(ModeNetwork)`)
- `getProviderLogs()`

Replace with `konveyor.NewAnalyzer()` pipeline:
- Rule loading, engine creation, rule execution, dependency output

Keep after analyzer returns:
- `CreateJSONOutput()`
- `GenerateStaticReport()`

**Step 5: Replace `runLocal()` in `pkg/testing/runner.go` with in-process analyzer**

The test command's local mode currently shells out to `kantra analyze` as a subprocess (`runner.go:317-351`):

```go
// Current: subprocess invocation
cmd := exec.Command(execPath, "analyze", "--run-local", "--skip-static-report",
    "--input", input, "--output", dir+"/output", "--rules", dir+"/rules.yaml",
    "--overwrite", "--enable-default-rulesets=false")
cmd.Run()
// Then reads output.yaml from disk and unmarshals it
```

Replace with direct `konveyor.Analyzer` call:

```go
// New: in-process analysis
func runInProcess(ctx context.Context, log logr.Logger, tempDir string,
    analysisParams AnalysisParams, input string) ([]konveyor.RuleSet, error) {

    settingsPath := filepath.Join(tempDir, "provider_settings.json")
    rulesPath := filepath.Join(tempDir, "rules.yaml")

    analyzer, err := konveyor.NewAnalyzer(
        konveyor.WithProviderConfigFilePath(settingsPath),
        konveyor.WithRuleFilepaths([]string{rulesPath}),
        konveyor.WithLogger(log),
        konveyor.WithContext(ctx),
    )
    if err != nil {
        return nil, err
    }
    defer analyzer.Stop()

    if _, err := analyzer.ParseRules(); err != nil {
        return nil, err
    }
    if err := analyzer.ProviderStart(); err != nil {
        return nil, err
    }

    return analyzer.Run(), nil
}
```

This eliminates:
- The `os.Executable()` path detection (`runner.go:336-338`)
- The `envWithoutKantraDir()` hack (`runner.go:349`, `354-365`)
- Reading and unmarshaling `output.yaml` from disk (results come back as Go structs)
- Subprocess overhead per test group

Update `ensureProviderSettings()` to use `DefaultProviderConfig(ModeLocal)` instead of the local `defaultProviderConfig` var.

The container path (`runInContainer`, `runner.go:367-425`) remains unchanged -- it runs `konveyor-analyzer` inside a container for integration-level testing.

**Step 6: Delete dead code**

Methods that become dead in the analyze command:

| Method | File | Lines |
|--------|------|-------|
| `setupProgressReporter()` | `analyze.go:1457-1556` | 100 |
| `renderProgressBar()` | `analyze-bin.go:62-79` | 18 |
| `ConsoleHook` struct + methods | `analyze-bin.go:40-59` | 20 |
| `DependencyOutputContainerless()` | `analyze-bin.go:847-906` | 60 |
| `setInternalProviders()` | `analyze-bin.go:787-811` | 25 |
| `startProvidersContainerless()` | `analyze-bin.go:813-845` | 33 |
| `setConfigsContainerless()` | `analyze-bin.go:613-659` | 47 |
| `setBuiltinProvider()` | `analyze-bin.go:661-682` | 22 |
| `setJavaProvider()` | `analyze-bin.go:684-699` | 16 |

Code that becomes dead in the test runner:

| Code | File | Lines |
|------|------|-------|
| `defaultProviderConfig` var | `pkg/testing/runner.go:46-126` | 80 |
| `runLocal()` function | `pkg/testing/runner.go:317-351` | 35 |
| `envWithoutKantraDir()` function | `pkg/testing/runner.go:354-365` | 12 |

**Step 7: Verify**

- `go build ./...`
- `go test ./...`
- Run `kantra test` with a sample `.test.yaml` in local mode to verify the in-process path
- Run integration tests if available

### Expected line count impact

| File | Before | After (estimate) |
|---|---|---|
| `analyze-bin.go` | 1,019 | ~400 |
| `analyze-hybrid.go` | 946 | ~500 |
| `analyze.go` | 1,600 | ~1,400 (dead code removed) |
| `pkg/testing/runner.go` | 585 | ~460 (subprocess code removed, shared defaults used) |
| `pkg/provider/defaults.go` | 0 | ~100 (new, shared provider config) |
| **Total delta** | | ~1,400 lines eliminated |

~1,400 lines eliminated, plus the duplication problem is solved at the architectural level for both the analyze and test commands.

---

## Phase 3b: Reorganize Into `cmd/analyze/` Sub-Package

After Phase 3a, the remaining code is leaner. Now reorganize into a proper sub-package with enforced boundaries.

### Prerequisite: Extract Settings (Done)

Moved `Config`, `Settings`, and all build-time variables from `cmd/settings.go` and `cmd/version.go`
into `cmd/internal/settings/settings.go`. This breaks the circular import that would otherwise occur
between `cmd/root.go` (imports `cmd/analyze`) and `cmd/analyze/` (needs `Settings`). All source files,
test files, and Dockerfile ldflags updated. Tests in `cmd/internal/settings/settings_test.go`.

### New package structure

```
cmd/
  root.go                    (imports cmd/analyze and cmd/internal/settings)
  test.go                    (unchanged -- thin shim, now uses in-process analyzer)
  
  internal/
    settings/
      settings.go            (~155 lines) Config, Settings, build vars, load methods
      settings_test.go       (~300 lines) Config tests (moved from cmd/settings_test.go)

  analyze/
    command.go               (~200 lines) NewAnalyzeCmd, Options, State, RunE routing
    validate.go              (~300 lines) Validate, CheckOverwriteOutput, ValidateContainerless, etc.
    labels.go                (~200 lines) Label fetching/listing (container + containerless)
    provider_config.go       (~300 lines) Settings generation, merging, override loading
    container_ops.go         (~400 lines) Volume, network, container management
    rules.go                 (~150 lines) Rule volume setup, extraction, temp rulesets
    containerless.go         (~150 lines) Containerless entry point + validation
    hybrid.go                (~350 lines) Hybrid entry point + container orchestration
    output.go                (~250 lines) JSON output, moveResults
    static_report.go         (~120 lines) Static HTML report generation (moved from cmd/static-report.go)
    progress.go              (~80 lines)  ProgressMode only (progress pipeline now in konveyor)
    cleanup.go               (~160 lines) All cleanup methods
    profile.go               (~80 lines)  Profile loading and applying

pkg/
  provider/
    defaults.go              (~100 lines) Shared DefaultProviderConfig(mode) (NEW from Phase 3a)
```

Note: `cmd/static-report.go` moves into `cmd/analyze/static_report.go` because it is only consumed by the analyze command (`buildStaticReportFile` calls `validateFlags`, `loadApplications`, `generateJSBundle`). It has no reason to be in the top-level `cmd` package.

### Struct decomposition

Split `analyzeCommand` (49 fields) into two structs:

```go
// Options holds CLI flags -- immutable after parsing
type Options struct {
    Input, Output, Mode      string
    Sources, Targets         []string
    LabelSelector            string
    Rules                    []string
    // ... all 38 flag fields
}

// State holds runtime state -- built during execution
type State struct {
    Log                    logr.Logger
    IsFileInput            bool
    SourceLocationPath     string  // from Phase 2
    ProvidersMap           map[string]ProviderInit
    ProviderContainerNames map[string]string
    TempDirs               []string
    NetworkName            string
    VolumeName             string
    MavenCacheVolumeName   string
    KantraDir              string
    NeedsBuiltin           bool
    TempRuleDir            string
    ReqMap                 map[string]string
}
```

### Consolidate the two `mergeProviderSpecificConfig` functions

- `analyze.go:1365` -- complex method (maven settings file copying, protected keys) → rename to `mergeProviderUserOptions` in `provider_config.go`
- `analyze-hybrid.go:104` -- simple map merge for overrides → rename to `mergeOverrideConfig` in `provider_config.go`

### Migration order (least-coupled first)

1. `progress.go` -- `ProgressMode` has zero method dependencies
2. `cleanup.go` -- only depends on state fields and `Settings`
3. `output.go` -- depends on options/state, not other methods
4. `labels.go` -- depends on options and container ops
5. `rules.go` -- depends on options and file ops
6. `validate.go` -- depends on labels, rules, profiles
7. `provider_config.go` -- provider config generation and merging
8. `container_ops.go` -- volume/network management
9. `profile.go` -- profile loading
10. `containerless.go` -- containerless entry point
11. `hybrid.go` -- hybrid entry point
12. `command.go` -- `NewAnalyzeCmd`, flag setup, `RunE` routing
13. Update `cmd/root.go` to import from `cmd/analyze`
14. Rewrite tests to use new structure

Each step: compile, run tests, commit independently.

---

## Testing Strategy

Tests that directly construct `analyzeCommand{}` will need rewriting:
- `cmd/analyze_test.go` (2,120 lines) -- constructs `analyzeCommand{}` in ~15 test functions
- `cmd/analyze-bin_test.go` (1,243 lines) -- constructs `analyzeCommand{}` in ~10 test functions
- `cmd/hybrid_integration_test.go` (308 lines) -- constructs `analyzeCommand{}` in 3 test functions
- `cmd/analyze_hybrid_inprocess_test.go` (294 lines) -- tests hybrid mode

Approach:
- Move tests alongside their source files into `cmd/analyze/`
- Update struct references from `analyzeCommand{}` to `Options{} + State{}`
- Tests that exercise the full `RunE` path can use `NewAnalyzeCmd()` and invoke the command

---

## Other Commands Assessed

The following commands were evaluated for similar refactoring needs:

| Command | Lines | Verdict |
|---------|-------|---------|
| `cmd/test.go` + `pkg/testing/` | 58 + 1,382 | **Included in Phase 3a** -- `runLocal()` replaced by in-process `konveyor.Analyzer`; shared provider defaults extracted |
| `cmd/openrewrite.go` | 198 | No change needed -- right-sized and self-contained |
| `cmd/transform.go` | 18 | No change needed -- proper parent command |
| `cmd/dump-rules.go` | 148 | No change needed -- simple utility |
| `cmd/version.go` | 30 | No change needed |
| `cmd/static-report.go` | 123 | **Moved in Phase 3b** -- only consumed by analyze, relocate to `cmd/analyze/` |
| `cmd/asset_generation/` | ~738 total | No change needed -- already properly sub-packaged |

---

## Dependencies Between Phases

```
Phase 2 (SourceMountPath)
    ↓
Phase 3a (Adopt konveyor.Analyzer)  ← requires analyzer-lsp PR #1033 merged
    │
    ├── Rewrite analyze containerless + hybrid execution
    ├── Replace test command runLocal() with in-process analyzer
    └── Extract shared provider defaults to pkg/provider/defaults.go
    ↓
Phase 3b (Reorganize into cmd/analyze/)
    │
    ├── Split analyzeCommand into Options + State
    ├── Move methods into focused files in cmd/analyze/
    ├── Move static-report.go into cmd/analyze/
    └── Rewrite tests for new structure
```

Phase 3b can be started independently of PR #1033 if needed, but file sizes and method counts will be larger without Phase 3a.
