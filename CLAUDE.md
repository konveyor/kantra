# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

Kantra is a CLI that unifies analysis and transformation capabilities of Konveyor. The CLI is built with Cobra. Podman or Docker is required for container-based workflows.

## Reference files (import for full context)

- @.claude/commands.md — Command references
- @.claude/testing.md — Testing commands and scenarios
- @.claude/architecture.md — Containerless vs hybrid architecture, repository layout
- @.claude/context.md — Component details, rules, analysis flow

## Kantra analysis modes:

### Hybrid mode (`--run-local=false`):

- Parses and validates flags and options, then drives the sequence: run Alizer for language detection, create the shared volume, extract rulesets, and start provider containers per discovered language (except the builtin provider), and run the analyzer-lsp engine on host. 

### Containerless mode (default, `--run-local=true`): 

- Parses and validates flags and options; Only the **Java provider** is supported. The analyzer-lsp engine runs on the host with the builtin provider and the Java provider in-process. Kantra loads default rulesets from disk (and/or `--rules`) and then runs the engine.

## Components (see `.claude/context.md` for details):

### Core Dependencies

Kantra orchestrates analysis but delegates work to external components.

- **[analyzer-lsp](https://github.com/konveyor/analyzer-lsp)** — Rule engine that performs the analysis. It loads rules (from default rulesets and/or `--rules`), runs rule evaluation and source and dependency analysis, and coordinates providers. The **[builtin provider](https://github.com/konveyor/analyzer-lsp/tree/main/provider/internal/builtin)** (in-process; evaluates builtin rules) and external providers. Produces the incident and dependency output. 

- **[External providers](https://github.com/konveyor/analyzer-lsp/tree/main/external-providers)** — Per-language providers (Java, Go, Python, NodeJS, C#) that implement LSP-based analysis for rule conditions. In hybrid mode kantra starts one container per detected language. In containerless mode only the Java provider is used (in-process).  

- **[Default rulesets](https://github.com/konveyor/rulesets)** — Curated rule definitions (e.g. Java migration rules) from [konveyor/rulesets](https://github.com/konveyor/rulesets/tree/main/stable). Shipped with kantra and passed to the analyzer-lsp engine; use `--rules` to append custom rules.

### Analyzer-lsp Analysis Modes

- **`--mode source-only`** - Analyzes only application source code
- **`--mode full`** - Source + dependency analysis 
  - Only the **Java provider** implements dependency analysis today

### Rules (See `.claude/context.md` for detailed rule structure, example rule, and how rules are evaluated)

Rules are YAML definitions that specify migration patterns. Analyzer-lsp is fundamentally a rules engine.

- **Default rulesets**: Automatically loaded from [konveyor/rulesets](https://github.com/konveyor/rulesets)
- **Custom rules**: Via `--rules=<path>` flag
- **Filtering**: Use `--target=<name>`, `--source=<name>`, or `--label-selector=<expr>` to select which rules run
- Rule writing: `docs/rules-quickstart.md`

## Development

### Build & Test
```bash
# Build binary
go build -o kantra .

# Run all tests
go test ./...

# Quick smoke test
./kantra analyze \
  --input pkg/testing/examples/ruleset/test-data/java \
  --output ./test-output \
  --target cloud-readiness \
  --overwrite
```

See `.claude/testing.md` for comprehensive testing scenarios.

## Additional Resources

- **Hybrid mode deep dive**: `docs/hybrid.md`
- **Containerless mode**: `docs/containerless.md`
- **Rule writing guide**: `docs/rules-quickstart.md`
- **Testing rules**: `docs/testrunner.md`
- **Usage examples**: `docs/examples.md`
- **Local testing commands**: `.claude/testing.md`
