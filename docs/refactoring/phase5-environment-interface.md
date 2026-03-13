# Phase 5: Provider Environment Interface

## Prerequisites

- **Phase 3a completed** -- `konveyor.Analyzer` adopted; both `RunAnalysisContainerless` and `RunAnalysisHybridInProcess` use `NewAnalyzer` / `ParseRules` / `ProviderStart` / `Run`
- **Phase 3b package move completed** -- Code lives in `cmd/analyze/` sub-package with `pkg/provider/defaults.go` providing shared `DefaultProviderConfig(mode, opts)`

## Problem

Despite Phase 3a eliminating the low-level engine duplication, `RunAnalysisContainerless` (~290 lines, `cmd/analyze/containerless.go:85`) and `RunAnalysisHybridInProcess` (~350 lines, `cmd/analyze/hybrid.go:155`) remain **~80% identical**. They follow the same 15-step structure but branch at 4 points:

| Step | Containerless | Hybrid |
|------|--------------|--------|
| Infrastructure setup | Validate java/mvn/JAVA_HOME | Create volume, extract rulesets, start provider containers, health checks |
| Provider config generation | `DefaultProviderConfig(ModeLocal, ...)` | `DefaultProviderConfig(ModeNetwork, ...)` with addresses |
| Config delivery to analyzer | `WithProviderConfigFilePath(settingsPath)` | `WithProviderConfigs(configs)` |
| Cleanup | Remove jdtls Eclipse dirs | Stop/remove containers, volumes, network |

Everything else -- progress setup, tracing, logging, override loading, label selectors, analyzer creation, rule parsing, execution, dependency resolution, output writing, static report -- is structurally identical.

The same duplication exists in the test runner (`pkg/testing/runner.go`): `runLocal()` and `runInContainer()` duplicate the analysis pipeline with different provider setup. The test runner cannot currently run in hybrid mode (providers in containers, analyzer on host), limiting it to Java-only containerless or all-in-one container.

Additionally, `--list-sources` and `--list-targets` have three separate code paths (`fetchLabels`, `fetchLabelsContainerless`, `readRuleFilesForLabels`) that all do the same thing -- walk rule files and text-grep for label strings -- differing only in where rule files are located. These labels are not project-aware (they dump all labels from all rulesets regardless of what providers apply to the project) and not capability-filtered (they don't consult providers about what rules they can evaluate).

## Design

### Core Abstraction: `provider.Environment` Interface

The `Environment` interface captures what varies between execution modes. It lives in `pkg/provider/` alongside the existing `Provider` interface and `DefaultProviderConfig` factory because it's fundamentally about provider lifecycle management.

Two complementary interfaces in the same package:
- `Provider` = "what config does one provider type need?" (existing)
- `Environment` = "how do we run providers collectively?" (new)

```go
// pkg/provider/environment.go

// Environment manages the lifecycle of analysis providers.
// Implementations handle the differences between running providers
// locally (containerless) vs in containers (hybrid). The caller
// creates an Environment via NewEnvironment and interacts with it
// through this interface without knowing which mode is active.
type Environment interface {
    // Start sets up provider infrastructure.
    //   Local: validates java/mvn/JAVA_HOME, checks .kantra directory
    //   Container: creates volumes, starts provider containers, health checks
    Start(ctx context.Context) error

    // Stop tears down provider infrastructure.
    //   Local: removes Eclipse jdtls temp directories
    //   Container: stops/removes provider containers, removes volumes
    Stop(ctx context.Context) error

    // ProviderConfigs returns the provider configurations for this environment.
    // Must be called after Start.
    //   Local: ModeLocal configs with host binary paths
    //   Container: ModeNetwork configs with localhost:PORT addresses
    ProviderConfigs() []provider.Config

    // Rules returns rule file/directory paths for analysis.
    //   Local: kantraDir/rulesets + user rules
    //   Container: rulesets extracted from container image + user rules
    Rules(userRules []string, enableDefaults bool) ([]string, error)

    // ExtraAnalyzerOptions returns mode-specific analyzer options.
    //   Local: none
    //   Container: path mappings for binary analysis
    ExtraAnalyzerOptions(params AnalysisParams) ([]core.AnalyzerOption, error)

    // PostAnalysis runs after analysis completes.
    //   Local: no-op
    //   Container: collects provider container logs
    PostAnalysis(ctx context.Context) error
}
```

### Factory

```go
// pkg/provider/environment.go

// NewEnvironment creates an Environment for the given configuration.
// The Mode field determines the implementation; the caller interacts
// only through the Environment interface.
func NewEnvironment(cfg EnvironmentConfig) Environment {
    switch cfg.Mode {
    case ModeLocal:
        return newLocalEnvironment(cfg)
    case ModeNetwork:
        return newContainerEnvironment(cfg)
    default:
        return newContainerEnvironment(cfg)
    }
}
```

Both `localEnvironment` and `containerEnvironment` are unexported. The caller never sees them.

### EnvironmentConfig

```go
// pkg/provider/environment.go

type EnvironmentConfig struct {
    // Mode determines which Environment implementation is created.
    Mode ExecutionMode

    // Shared fields
    Input            string
    IsFileInput      bool
    AnalysisMode     string
    ContextLines     int
    MavenSettingsFile string
    JvmMaxMem        string
    HTTPProxy        string
    HTTPSProxy       string
    NoProxy          string
    Log              logr.Logger

    // Local mode fields (ignored in container mode)
    KantraDir          string
    DisableMavenSearch bool

    // Container mode fields (ignored in local mode)
    Providers            []ProviderInfo
    ContainerBinary      string
    RunnerImage          string
    OutputDir            string
    EnableDefaultRulesets bool
    LogLevel             *uint32
    Cleanup              bool
    DepFolders           []string
}

type ProviderInfo struct {
    Name  string
    Image string
}

type AnalysisParams struct {
    IsBinaryAnalysis bool
    Input            string
}
```

### No Orchestrator Package

The analyzer-lsp API calls are 5 lines:

```go
anlzr, _ := core.NewAnalyzer(opts...)
anlzr.ParseRules()
anlzr.ProviderStart()
rulesets := anlzr.Run()
anlzr.Stop()
```

This is not duplication worth extracting. Each caller (analyze command, test runner, label listing) has different concerns around these calls (progress reporting, output writing, test verification). The `Environment` interface eliminates the real duplication -- infrastructure lifecycle, config generation, rule sourcing -- which accounts for ~200+ lines per mode.

---

## How Callers Use the Environment

### `cmd/analyze/` -- Analysis (both modes, no branching)

The mode branching in `RunE` collapses:

```go
// Mode decision happens once, early
mode := provider.ModeLocal
if len(foundProviders) > 0 && !slices.Contains(foundProviders, util.JavaProvider) {
    mode = provider.ModeNetwork
}
if !analyzeCmd.runLocal {
    mode = provider.ModeNetwork
}

if analyzeCmd.listSources || analyzeCmd.listTargets {
    return analyzeCmd.listLabels(ctx, mode, foundProviders)
}
return analyzeCmd.runAnalysis(ctx, mode, foundProviders)
```

`RunAnalysisContainerless` and `RunAnalysisHybridInProcess` are replaced by a single `runAnalysis`:

```go
func (a *analyzeCommand) runAnalysis(ctx context.Context, mode provider.ExecutionMode, foundProviders []string) error {
    // --- CLI concerns: progress, tracing, logging ---
    progressMode := NewProgressMode(a.noProgress)
    progressMode.HideCursor()
    defer progressMode.ShowCursor()

    if a.jaegerEndpoint != "" {
        tp, _ := tracing.InitTracerProvider(a.log, tracing.Options{...})
        defer tracing.Shutdown(ctx, a.log, tp)
    }

    analysisLogFile, _ := os.Create(filepath.Join(a.output, "analysis.log"))
    defer analysisLogFile.Close()
    // ... logrus setup ...

    // --- Create environment (mode-agnostic from here) ---
    env := provider.NewEnvironment(provider.EnvironmentConfig{
        Mode:                 mode,
        Input:                a.input,
        IsFileInput:          a.isFileInput,
        Providers:            buildProviderInfos(foundProviders),
        ContainerBinary:      settings.Settings.ContainerBinary,
        RunnerImage:          settings.Settings.RunnerImage,
        KantraDir:            a.kantraDir,
        AnalysisMode:         a.mode,
        // ... remaining config fields ...
    })

    if err := env.Start(ctx); err != nil {
        return err
    }
    defer env.Stop(ctx)
    progressMode.Printf("  ✓ Started providers\n")

    // --- Provider configs + overrides ---
    configs := env.ProviderConfigs()
    overrideConfigs, _ := a.loadOverrideProviderSettings()
    if overrideConfigs != nil {
        for i := range configs { configs[i] = applyProviderOverrides(configs[i], overrideConfigs) }
    }

    // --- Rules ---
    rules, _ := env.Rules(a.rules, a.enableDefaultRulesets)

    // --- Label selectors ---
    depLabelSelector := ""
    if !a.analyzeKnownLibraries {
        depLabelSelector = fmt.Sprintf("!%v=open-source", providerapi.DepSourceLabel)
    }

    // --- Progress reporter ---
    reporter, progressDone, progressCancel := setupProgressReporter(ctx, a.noProgress)
    if progressCancel != nil { defer progressCancel() }

    // --- Build analyzer options ---
    opts := []core.AnalyzerOption{
        core.WithProviderConfigs(configs),
        core.WithRuleFilepaths(rules),
        core.WithLabelSelector(a.getLabelSelector()),
        core.WithContextLinesLimit(a.contextLines),
        core.WithLogger(analyzeLog),
        core.WithContext(ctx),
        core.WithReporters(reporter),
    }
    if a.incidentSelector != "" {
        opts = append(opts, core.WithIncidentSelector(a.incidentSelector))
    }
    if a.noDepRules {
        opts = append(opts, core.WithDependencyRulesDisabled())
    }
    if depLabelSelector != "" {
        opts = append(opts, core.WithDepLabelSelector(depLabelSelector))
    }
    extraOpts, _ := env.ExtraAnalyzerOptions(provider.AnalysisParams{
        IsBinaryAnalysis: isBinaryAnalysis, Input: a.input,
    })
    opts = append(opts, extraOpts...)

    // --- Analyzer lifecycle ---
    anlzr, err := core.NewAnalyzer(opts...)
    if err != nil { return fmt.Errorf("failed to create analyzer: %w", err) }
    defer anlzr.Stop()

    progressMode.Printf("  ✓ Initialized providers\n")

    if _, err = anlzr.ParseRules(); err != nil {
        return fmt.Errorf("failed to parse rules: %w", err)
    }
    if err = anlzr.ProviderStart(); err != nil {
        return fmt.Errorf("failed to start providers: %w", err)
    }

    progressMode.Printf("  ✓ Started rules engine\n")

    rulesets := anlzr.Run()

    depFile := filepath.Join(a.output, "dependencies.yaml")
    if err := anlzr.GetDependencies(depFile, false); err != nil {
        a.log.Error(err, "failed to get dependencies")
    }

    if progressMode.IsEnabled() && progressCancel != nil {
        progressCancel()
        <-progressDone
    }

    env.PostAnalysis(ctx)

    // --- Write output ---
    sort.SliceStable(rulesets, func(i, j int) bool { return rulesets[i].Name < rulesets[j].Name })
    b, _ := yaml.Marshal(rulesets)
    os.WriteFile(filepath.Join(a.output, "output.yaml"), b, 0644)
    a.CreateJSONOutput()
    analysisLogFile.Close()
    a.GenerateStaticReport(ctx, a.log)

    progressMode.Println("\nResults:")
    progressMode.Printf("  Report: file://%s\n", filepath.Join(a.output, "static-report", "index.html"))
    progressMode.Printf("  Analysis logs: %s\n", filepath.Join(a.output, "analysis.log"))
    return nil
}
```

### `cmd/analyze/` -- Label listing (project-aware)

Three code paths collapse to one. Labels are filtered by active provider capabilities:

```go
func (a *analyzeCommand) listLabels(ctx context.Context, mode provider.ExecutionMode, foundProviders []string) error {
    env := provider.NewEnvironment(provider.EnvironmentConfig{
        Mode: mode, Input: a.input,
        Providers: buildProviderInfos(foundProviders),
        // ...
    })

    // Must start providers -- parser needs their capabilities to filter rules
    if err := env.Start(ctx); err != nil {
        return err
    }
    defer env.Stop(ctx)

    configs := env.ProviderConfigs()
    rules, _ := env.Rules(a.rules, a.enableDefaultRulesets)

    anlzr, _ := core.NewAnalyzer(
        core.WithProviderConfigs(configs),
        core.WithRuleFilepaths(rules),
        core.WithLogger(a.log),
        core.WithContext(ctx),
    )
    defer anlzr.Stop()

    parsedRules, _ := anlzr.ParseRules()
    extractAndPrintLabels(parsedRules, a.listSources, a.listTargets, os.Stdout)
    return nil
}
```

### `pkg/testing/runner.go` -- Test runner

```go
func (r defaultRunner) Run(testFiles []TestsFile, opts TestOptions) ([]Result, error) {
    if opts.Log.GetSink() == nil {
        opts.Log = logr.Discard()
    }

    allResults := []Result{}
    for idx := range testFiles {
        testsFile := testFiles[idx]
        testGroups := groupTestsByAnalysisParams(testsFile.Tests)
        results := []Result{}

        for _, tests := range testGroups {
            tempDir, err := os.MkdirTemp(opts.TempDir, "rules-test-")
            if err != nil {
                results = append(results, Result{TestsFilePath: testsFile.Path, Error: err})
                continue
            }

            logFile, err := os.OpenFile(filepath.Join(tempDir, "analysis.log"),
                os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
            if err != nil {
                results = append(results, Result{TestsFilePath: testsFile.Path, Error: err})
                continue
            }

            baseLogger := logrus.New()
            baseLogger.SetOutput(logFile)
            baseLogger.SetLevel(logrus.InfoLevel)
            logger := logrusr.New(baseLogger)

            if err = ensureRules(testsFile.RulesPath, tempDir, tests); err != nil {
                results = append(results, Result{TestsFilePath: testsFile.Path, Error: err})
                logFile.Close()
                continue
            }

            analysisParams := tests[0].TestCases[0].AnalysisParams
            dataPath := filepath.Join(filepath.Dir(testsFile.Path),
                filepath.Clean(testsFile.Providers[0].DataPath))

            // --- Determine mode ---
            mode := provider.ModeNetwork  // default: hybrid
            if opts.RunLocal {
                mode = provider.ModeLocal
            }

            kantraDir, _ := util.GetKantraDir()

            // --- Create environment ---
            env := provider.NewEnvironment(provider.EnvironmentConfig{
                Mode:            mode,
                Input:           dataPath,
                Providers:       buildTestProviderInfos(testsFile.Providers, opts),
                ContainerBinary: opts.ContainerBinary,
                KantraDir:       kantraDir,
                AnalysisMode:    analysisParams.Mode,
                Log:             logger,
            })

            // --- Start providers ---
            if err := env.Start(ctx); err != nil {
                results = append(results, Result{TestsFilePath: testsFile.Path, Error: err})
                logFile.Close()
                continue
            }

            // --- Provider configs ---
            configs := env.ProviderConfigs()

            // --- Build analyzer options ---
            analyzerOpts := []core.AnalyzerOption{
                core.WithProviderConfigs(configs),
                core.WithRuleFilepaths([]string{filepath.Join(tempDir, "rules.yaml")}),
                core.WithLogger(logger),
                core.WithContext(context.Background()),
                core.WithDependencyRulesDisabled(),
            }
            if analysisParams.DepLabelSelector != "" {
                analyzerOpts = append(analyzerOpts,
                    core.WithDepLabelSelector(analysisParams.DepLabelSelector))
            }

            // --- Analyzer lifecycle ---
            anlzr, err := core.NewAnalyzer(analyzerOpts...)
            if err != nil {
                env.Stop(ctx)
                results = append(results, Result{TestsFilePath: testsFile.Path, Error: err})
                logFile.Close()
                continue
            }

            anlzr.ParseRules()
            anlzr.ProviderStart()
            rulesets := anlzr.Run()

            anlzr.Stop()
            env.Stop(ctx)
            logFile.Close()

            // --- Verify test cases ---
            anyFailed := false
            groupResults := []Result{}
            for _, test := range tests {
                for _, tc := range test.TestCases {
                    result := Result{
                        TestsFilePath: testsFile.Path,
                        RuleID:        test.RuleID,
                        TestCaseName:  tc.Name,
                    }
                    if len(rulesets) > 0 {
                        result.FailureReasons = tc.Verify(rulesets[0])
                    } else {
                        result.FailureReasons = []string{"empty output"}
                    }
                    if len(result.FailureReasons) == 0 {
                        result.Passed = true
                    } else {
                        anyFailed = true
                        result.DebugInfo = append(result.DebugInfo,
                            fmt.Sprintf("find debug data in %s", tempDir))
                    }
                    groupResults = append(groupResults, result)
                }
            }
            results = append(results, groupResults...)
            if !anyFailed && !opts.NoCleanup {
                os.RemoveAll(tempDir)
            }
        }

        if opts.ProgressPrinter != nil {
            opts.ProgressPrinter(os.Stdout, results)
        }
        allResults = append(allResults, results...)
    }
    return allResults, nil
}
```

---

## Implementation Details

### New Files

**`pkg/provider/environment.go`** (~100 lines)
- `Environment` interface (6 methods)
- `EnvironmentConfig` struct
- `ProviderInfo` struct
- `AnalysisParams` struct
- `NewEnvironment(cfg)` factory function

**`pkg/provider/env_local.go`** (~130 lines)
- `localEnvironment` struct (unexported)
- `newLocalEnvironment(cfg)` constructor
- `Start`: validates java/mvn/JAVA_HOME, checks `.kantra` directory contents (moves from `cmd/analyze/validate.go:ValidateContainerless`)
- `Stop`: removes Eclipse jdtls temp dirs (moves from `cmd/analyze/cleanup.go:cleanlsDirs`)
- `ProviderConfigs`: calls `DefaultProviderConfig(ModeLocal, DefaultOptions{...})`
- `Rules`: returns `kantraDir/rulesets` + user rules
- `ExtraAnalyzerOptions`: returns nil
- `PostAnalysis`: returns nil

**`pkg/provider/env_container.go`** (~350 lines)
- `containerEnvironment` struct (unexported) -- holds volume name, container names, provider addresses, extracted rulesets dir
- `newContainerEnvironment(cfg)` constructor
- `Start`: creates container volume, extracts default rulesets, starts provider containers with port publishing, runs health checks (moves from `cmd/analyze/container_ops.go` and `cmd/analyze/hybrid.go`)
- `Stop`: stops/removes provider containers, removes volumes, removes network (moves from `cmd/analyze/cleanup.go`)
- `ProviderConfigs`: calls `DefaultProviderConfig(ModeNetwork, DefaultOptions{ProviderAddresses: ...})`
- `Rules`: returns extracted rulesets dir + user rules
- `ExtraAnalyzerOptions`: returns path mappings for binary analysis or `WithIgnoreAdditionalBuiltinConfigs` for source
- `PostAnalysis`: collects provider container logs

**`cmd/analyze/run.go`** (~150 lines)
- Single `runAnalysis` method using the environment (replaces both `RunAnalysisContainerless` and `RunAnalysisHybridInProcess`)

**`pkg/provider/env_local_test.go`** (~100 lines)
**`pkg/provider/env_container_test.go`** (~120 lines)

### Modified Files

**`cmd/analyze/containerless.go`** -- deleted (absorbed by `run.go` + `env_local.go`)
**`cmd/analyze/hybrid.go`** -- deleted (absorbed by `run.go` + `env_container.go`)
**`cmd/analyze/analyze.go`** -- mode branching simplified to `NewEnvironment(cfg)` + one call
**`cmd/analyze/labels.go`** -- 4 functions / 3 code paths → 1 function / 1 code path, project-aware
**`cmd/analyze/container_ops.go`** -- most functions move to `env_container.go`
**`cmd/analyze/cleanup.go`** -- most functions move to environment implementations
**`cmd/analyze/validate.go`** -- `ValidateContainerless` moves to `localEnvironment.Start`
**`cmd/analyze/context.go`** -- remove fields that moved to environments
**`cmd/analyze/rules.go`** -- `extractDefaultRulesets` moves to `containerEnvironment.Start`
**`pkg/testing/runner.go`** -- `runLocal`/`runInContainer`/`ensureProviderSettings`/`getMergedProviderConfig`/`defaultProviderConfig` removed, replaced by environment
**`cmd/test.go`** -- hybrid default, remove container-specific flags

### Files That Don't Change

| File | Reason |
|------|--------|
| `pkg/provider/provider.go` | `Provider` interface unchanged |
| `pkg/provider/defaults.go` | `DefaultProviderConfig`, `ExecutionMode` unchanged |
| `pkg/provider/java.go`, `go.go`, etc. | Individual provider configs unchanged |
| `pkg/provider/volume.go` | Volume utilities unchanged |
| `pkg/container/container.go` | Low-level container abstraction unchanged |
| `cmd/analyze/output.go` | Output writing unchanged |
| `cmd/analyze/progress.go` | Progress mode unchanged |

---

## Test Runner Mode Change

### Before
```
kantra test <file>
  ├── RunLocal=true  → runLocal()        → analyzer + providers in-process (Java only)
  └── RunLocal=false → runInContainer()  → everything in one container (all langs)
```

### After
```
kantra test <file>
  ├── --run-local    → localEnvironment    → analyzer + providers in-process (Java only)
  └── default        → containerEnvironment → analyzer in-process, providers in containers (all langs)
```

All-in-one container mode removed. Default becomes hybrid.

## Label Listing Change

### Before
```
--list-targets
  ├── runLocal=true  → walk kantraDir/rulesets, text-grep for label strings
  └── runLocal=false → run kantra inside a container, which walks /opt/rulesets
  → Returns ALL labels from ALL rulesets regardless of project
```

### After
```
--list-targets --input ./my-java-project
  1. Detect languages → [Java]
  2. Create environment (knows which providers apply)
  3. env.Start() → start providers (needed for capability reporting)
  4. Parse rules filtered by active provider capabilities
  5. Extract labels from parsed rules
  → Returns only labels from rules that match active providers
```

---

## Dependency Graph

```
cmd/analyze/      → pkg/provider/  (NewEnvironment, EnvironmentConfig)
                  → analyzer-lsp/core (NewAnalyzer, AnalyzerOption)

cmd/test.go       → pkg/provider/  (NewEnvironment, EnvironmentConfig)
pkg/testing/      → pkg/provider/  (NewEnvironment, EnvironmentConfig)
                  → analyzer-lsp/core (NewAnalyzer)

pkg/provider/     → pkg/container/ (containerEnvironment uses container.NewContainer)
                  → analyzer-lsp/provider (Config types -- existing dependency)
                  → (no new dependency on analyzer-lsp/core)
```

No circular dependencies. `pkg/provider/` does NOT import `analyzer-lsp/core`.

Settings values (`ContainerBinary`, `RunnerImage`, provider images) are passed via `EnvironmentConfig` fields rather than imported from `cmd/internal/settings/`.

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Container ops move from methods on `AnalyzeCommandContext` to `containerEnvironment`. They store state in `c.volumeName`, `c.providerContainerNames`, etc. | `containerEnvironment` struct holds this state. Clean 1:1 field mapping. |
| `WithProviderConfigFilePath` → `WithProviderConfigs` change in containerless mode. | Test runner already uses `WithProviderConfigs` for local mode (`runner.go:276`). No behavior change. |
| Test runner `runInContainer` removal -- losing the ability to run ALL analysis in one container. | Hybrid mode is functionally equivalent (same providers, same rules) but with the analyzer on host. |
| `settings.Settings` accessed directly in container ops. Moving to `pkg/provider/` would require importing `cmd/internal/settings/`. | Values passed as `EnvironmentConfig` fields. No hidden globals in `pkg/`. |
| `extractDefaultRulesets` uses `settings.Version` for cache directory naming. | Pass version as an `EnvironmentConfig` field. |
| Health check timeout (30s) is hardcoded in `waitForProvider`. | Make it an `EnvironmentConfig` field with a default. |

---

## Implementation Order

Single PR. Implementation follows this order to maintain compilability:

1. Create `pkg/provider/environment.go` -- interface, config types, factory
2. Create `pkg/provider/env_local.go` -- move validation and cleanup from `cmd/analyze/`
3. Create `pkg/provider/env_container.go` -- move container ops, health checks, cleanup from `cmd/analyze/`
4. Create `cmd/analyze/run.go` -- single `runAnalysis` method using the environment
5. Update `cmd/analyze/analyze.go` -- mode decision + single call to `runAnalysis`
6. Update `cmd/analyze/labels.go` -- single `listLabels` using environment + parsed rules
7. Delete `cmd/analyze/containerless.go` -- absorbed by `run.go` + `env_local.go`
8. Delete `cmd/analyze/hybrid.go` -- absorbed by `run.go` + `env_container.go`
9. Slim `cmd/analyze/container_ops.go` -- most moved to `env_container.go`
10. Slim `cmd/analyze/cleanup.go` -- most moved to environment implementations
11. Slim `cmd/analyze/context.go` -- remove fields that moved to environments
12. Update `pkg/testing/runner.go` -- replace `runLocal`/`runInContainer` with environment
13. Update `cmd/test.go` -- new flags, hybrid default
14. Update tests
15. Remove dead code (`ModeContainer`, `AllContainerProviders`, `defaultProviderConfig`, `ensureProviderSettings`, `getMergedProviderConfig`)

### Verification at each step

- `go build ./...`
- `go vet ./...`
- `go test ./...`

---

## Expected Impact

| Metric | Before | After |
|--------|--------|-------|
| `cmd/analyze/containerless.go` | ~290 lines | deleted |
| `cmd/analyze/hybrid.go` | ~350 lines | deleted |
| `cmd/analyze/run.go` | 0 | ~150 lines (new, unified) |
| `cmd/analyze/labels.go` | ~205 lines (4 functions, 3 code paths) | ~60 lines (1 function, 1 code path) |
| `cmd/analyze/container_ops.go` | ~320 lines | ~50 lines (utilities only) |
| `cmd/analyze/cleanup.go` | ~158 lines | ~40 lines (StopProvider only) |
| `pkg/provider/environment.go` | 0 | ~100 lines (new) |
| `pkg/provider/env_local.go` | 0 | ~130 lines (new) |
| `pkg/provider/env_container.go` | 0 | ~350 lines (new) |
| `pkg/testing/runner.go` | ~549 lines | ~300 lines |
| Mode branching in callers | if/else at 6+ locations | `NewEnvironment(cfg)` -- zero branching |
| Test runner modes | container (all-in-one) + local | hybrid + local |
| `--list-targets` accuracy | all labels, text-grepped | project-aware, capability-filtered, properly parsed |
| **Net line delta** | | ~-400 lines, significant complexity reduction |
