# Kantra Analysis Components

**Hybrid mode** (`--run-local=false`): Kantra parses flags and options (e.g. `--input`, `--output`, `--rules`, profile), validates paths and settings, then drives the sequence: run Alizer for language detection, create the shared volume, extract default rulesets and start provider containers, set up provider clients and the builtin provider, run the analyzer-lsp engine, write `output.yaml` and `dependencies.yaml` and optionally the static HTML report, and finally clean up containers and temp resources. It does not perform analysis itself; it configures and invokes the engine and providers.

**Containerless mode** (default): Kantra parses flags and options; Only the **Java provider** is supported. The analyzer-lsp engine runs on the host with the builtin provider and the Java provider in-process (see `cmd/analyze-bin.go`). Kantra loads default rulesets from disk (and/or `--rules`), runs the engine, and writes `output.yaml`, `dependencies.yaml`, and optionally the static HTML report. For other languages, use hybrid mode.

### [Alizer](https://github.com/devfile/alizer)

Application analyzer toolkit (Go library) used to detect **languages** in the input path. Kantra uses this to decide which **provider containers** to start — e.g. Java, Go, Python, NodeJS, C# 

### [analyzer-lsp](https://github.com/konveyor/analyzer-lsp) 

The rule engine that performs the actual analysis. It loads rules (from default rulesets and/or `--rules`), runs rule evaluation and dependency analysis, and coordinates all providers: the **builtin provider** (in-process) and **external provider clients** (gRPC). It produces the incident and dependency output that kantra writes to disk. Lives under [engine](https://github.com/konveyor/analyzer-lsp/tree/main/engine); rules are parsed by the [parser](https://github.com/konveyor/analyzer-lsp/tree/main/parser). 

### [Builtin provider](https://github.com/konveyor/analyzer-lsp/tree/main/provider/internal/builtin)

An in-process provider with analyzer-lsp. It implements the same provider interface as the language-specific providers. Used for builtin rule evaluation.

### [External providers](https://github.com/konveyor/analyzer-lsp/tree/main/external-providers)

gRPC providers. Each language (Java, Go, Python, NodeJS, C#) has a corresponding external provider; in hybrid mode kantra starts one container per detected language. The external provider use language server protocol for analyzing rule conditions.

### [java-analyzer-bundle](https://github.com/konveyor/java-analyzer-bundle) (Java provider base)

For the **Java provider*. It is an add-on for [Eclipse JDT Language Server (jdt.ls)](https://github.com/eclipse-jdtls/eclipse.jdt.ls) that extends workspace search with more parameters used by analyzer-lsp for Java analysis. 

### [Default rulesets](https://github.com/konveyor/rulesets/tree/main/stable)

Curated rule definitions (e.g. Java migration rules) shipped with kantra. They are passed to the analyzer-lsp engine so the engine can evaluate them against the application.

## Rules

Rules are YAML definitions that specify migration patterns. Analyzer-lsp is fundamentally a rules engine.

**What Rules Do**:
- Define a **condition** 
- When condition is true, create a **violation/incident** 
- Violations include: file location, line number, message, category, effort estimate

**Rule Sources**:
- **Default rulesets**: Automatically loaded from [konveyor/rulesets](https://github.com/konveyor/rulesets)
- **Custom rules**: Via `--rules=<path>` flag

**Filtering**:
- `--target=<target>` - e.g., `quarkus`, `eap8`, `cloud-readiness`
- `--source=<source>` - e.g., `eap7`, `springboot`
- `--label-selector=<expr>` - Advanced filtering (e.g., `!konveyor.io/source=open-source`)

**Documentation**:
- Writing rules: `docs/rules-quickstart.md`
- Testing rules: `docs/testrunner.md` and `.claude/testing.md`
- analyzer-lsp rules: [analyzer-lsp/docs/rules.md](https://github.com/konveyor/analyzer-lsp/blob/main/docs/rules.md)

### Rule Structure Example

```yaml
ruleID: local-storage-00001                    # Unique identifier 
category: mandatory                            # Severity
effort: 1                                   
labels:                                      
  - konveyor.io/target=cloud-readiness        # This rule applies when --target=cloud-readiness or label-selector
  - konveyor.io/source                      
  - storage                               
description: File system - Java IO           
message: |-                                   
  An application running inside a container could lose access to a file in local storage.
when:                                          # The condition - when this is true, violation is created
  or:                                          # Logical OR - any of these conditions trigger the rule
    - java.referenced:                         # Java-specific condition (requires Java provider)
        location: CONSTRUCTOR_CALL             # Look for constructor calls
        pattern: java.io.(FileWriter|FileReader|PrintStream|File|PrintWriter|RandomAccessFile)* # Regex pattern for searching
    - java.referenced:
        location: CONSTRUCTOR_CALL
        pattern: java.util.zip.ZipFile*
```

---

