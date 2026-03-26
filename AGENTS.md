# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

Kantra is a CLI that unifies analysis and transformation capabilities of Konveyor. The CLI is built with Cobra. Podman or Docker is required for container-based workflows.

## Reference files (import for full context)

- @.claude/commands.md — Command references
- @.claude/testing.md — Testing commands and scenarios
- @.claude/architecture.md — Containerless vs hybrid architecture, repository layout
- @.claude/context.md — Component details, rules, analysis flow
- @.claude/skills.md — Project skills (e.g. testing local code changes)

## Portable agent instruction (plain text)

- If you load this file, also read: `.claude/commands.md` - Command references
- If you load this file, also read: `.claude/testing.md` - Testing commands and scenarios
- If you load this file, also read: `.claude/architecture.md` - Containerless vs hybrid architecture, repository layout
- If you load this file, also read: `.claude/context.md` - Component details, rules, analysis flow
- If you load this file, also read: `.claude/skills.md` - Project skills (e.g. testing local code changes)

## Kantra analysis modes:

### Hybrid mode (`--run-local=false`):

- Parses and validates flags and options, then drives the sequence: run Alizer for language detection, create the shared volume, extract rulesets, and start provider containers per discovered language (except the builtin provider), and run the analyzer-lsp engine on host. 

### Containerless mode (default, `--run-local=true`): 

- Parses and validates flags and options; Only the **Java provider** and **builtin provider** are supported. The analyzer-lsp engine runs on the host with the builtin provider and the Java provider in-process. Kantra loads default rulesets from disk (and/or `--rules`) and then runs the engine.

## Components

*Full details: `.claude/context.md`*

Kantra orchestrates analysis but delegates work to external components.

- **[Alizer](https://github.com/devfile/alizer)** — Detects languages in the input path; kantra uses this to decide which provider containers to start (hybrid mode).

- **[analyzer-lsp](https://github.com/konveyor/analyzer-lsp)** — Rule engine: loads rules, runs evaluation and dependency analysis, coordinates providers. Produces incident and dependency output. **[Builtin provider](https://github.com/konveyor/analyzer-lsp/tree/main/provider/internal/builtin)** runs in-process for builtin rules.

- **[External providers](https://github.com/konveyor/analyzer-lsp/tree/main/external-providers)** — Per-language (Java, Go, Python, NodeJS, C#); LSP-based rule conditions. Hybrid: one container per language; containerless: Java provider in-process only.

- **[java-analyzer-bundle](https://github.com/konveyor/java-analyzer-bundle)** — Java provider base (JDT LS add-on) used for Java analysis.

- **[Default rulesets](https://github.com/konveyor/rulesets)** — Curated rules from [konveyor/rulesets](https://github.com/konveyor/rulesets/tree/main/stable); use `--rules` to append custom rules.

### Analyzer-lsp Analysis Modes

- **`--mode source-only`** - Analyzes only application source code
- **`--mode full`** - Source + dependency analysis 
  - Only the **Java provider** implements dependency analysis today

### Rules

Rules are YAML definitions: a **condition** (`when`, e.g. `java.referenced` with `location` and `pattern`) that when true produces a **violation** (file, line, message, category, effort). Default rulesets are loaded automatically; add custom rules with `--rules=<path>`. Filter with `--target`, `--source`, or `--label-selector`. Rule writing: `docs/rules-quickstart.md`.

*Full rule structure and example: `.claude/context.md`*

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
