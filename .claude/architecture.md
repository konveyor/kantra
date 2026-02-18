# Architecture Overview

## Analyze - Hybrid Mode

Kantra analyze **container (hybrid) mode** is activated with `kantra analyze --input=<path> --output=<path> --run-local=false`. The analyzer runs in-process on the host; external providers run in containers. 

### High-Level Architecture 

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Host (kantra process)                         │
├─────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │                    analyzer-lsp engine                           │  │
│  │         (rules parsing, rule evaluation, builtin provider)          │  │
│  └──────────────────────────────────┬──────────────────────────────┘  │
│                                     │                                 │
└─────────────────────────────────────┼─────────────────────────────────┘
                                │
                ┌───────────────┴───────────────┐
                │                               │
                ▼                               ▼
        ┌───────────────┐               ┌───────────────┐
        │ Java provider │               │  Go provider  │
        │  container    │               │  container    │
        │ (gRPC server  │   ...         │ (gRPC server  │
        │  on :PORT)     │               │  on :PORT)     │
        └───────────────┘               └───────────────┘
```


## Analyze - Containerless Mode

Containerless mode is the default: `kantra analyze --input=<path> --output=<path>`. Only the **Java provider** is supported; it runs on the host. For other languages (Go, Python, NodeJS, C#) use hybrid mode (`--run-local=false`).

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Host (kantra process)                         │
├─────────────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │                    analyzer-lsp engine                           │  │
│  │   (rules parsing, builtin provider, Java provider)              │  │
│  └─────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

## Component Layers

| Component | Purpose |
|-----------|---------|
| **kantra CLI** | Parses flags, orchestrates flow: [Alizer](https://github.com/devfile/alizer), volume creation, container start/stop, provider client setup, engine run, output writing. |
| **[analyzer-lsp](https://github.com/konveyor/analyzer-lsp/tree/main/engine) engine** | Runs rule evaluation, source and dependency analysis, and coordinates providers (builtin + network). |
| **[Builtin provider](https://github.com/konveyor/analyzer-lsp/tree/main/provider/internal/builtin)** | In-process provider that evaluates builtin-rules
| **[External providers](https://github.com/konveyor/analyzer-lsp/tree/main/external-providers)** | gRPC clients; evaluates respective rules; i.e. `<provider>.referenced` |
| **[Default rulesets](https://github.com/konveyor/rulesets/tree/main/stable)** | Rules provided with kantra; Use --rules to append more rules for analysis |


## Relevant Code

- **Entry**: `cmd/analyze.go` (RunE) — branch `runLocal == true` → containerless; `runLocal == false` → `RunAnalysisHybridInProcess()`.
- **Containerless flow**: `cmd/analyze-bin.go` — source and binary analysis in containerless mode (Java provider in-process).
- **Hybrid flow**: `cmd/analyze-hybrid.go` — `RunAnalysisHybridInProcess()`, `setupNetworkProvider()`, `setupBuiltinProviderHybrid()`.
- **Container lifecycle**: `cmd/analyze.go` — `RunProvidersHostNetwork()` (start containers with port publish and volumes); cleanup in `cmd/cleanup.go`.
- **Engine**: [konveyor/analyzer-lsp](https://github.com/konveyor/analyzer-lsp) — rule engine and LSP-based analysis.

## Directory Structure

- `cmd/` — Cobra subcommands: `analyze.go`, `analyze-bin.go`, `analyze-hybrid.go`, `transform`, `test`, and `config` (sync, login, list)
  - `cmd/internal/` — Command-specific helpers
  - `cmd/config/` — kantra config subcommand
- `pkg/` — Shared packages:
  - `pkg/provider/` — Provider implementations (Java, Go, C#, Node.js, Python)
  - `pkg/container/` — Container runtime abstractions
  - `pkg/profile/` — Analysis profiles
  - `pkg/testing/` — Test runner for YAML rules
  - `pkg/util/` — Common utilities
- `internal/constants/` — Internal constants
- `hack/` — Development scripts (setup, release helpers)
- `docs/` — Documentation (`hybrid.md`, `containerless.md`, `examples.md`, `testrunner.md`)
- `main.go` — Entrypoint; calls `cmd.Execute()`
