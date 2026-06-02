# Kantra usage guide

Kantra is a CLI for [Konveyor](https://konveyor.io/) static analysis and code transformation. This guide covers day-to-day `kantra analyze` usage: execution modes, language providers, rules, filtering, and output. For install steps and quick starts, see the [README](../README.md).

## Analysis modes

Kantra runs the analyzer on the host. Language providers either run in-process (containerless) or in containers (hybrid).

| Mode | How to run | Providers | When to use |
|------|------------|-----------|-------------|
| **Containerless** (default) | `kantra analyze --input ... --output ...` | Java + builtin in-process | Local Java work, fastest path on Linux when JDK/Maven are installed |
| **Hybrid** | Add `--run-local=false` | Per-language containers over the network | Go, Python, Node.js, C#, multi-language apps, macOS (better I/O than full containerization) |

- **Containerless prerequisites** (OpenJDK 17+, Maven, `JAVA_HOME`, optional Java 8 for Gradle): [containerless.md](containerless.md)
- **Hybrid setup** (Podman/Docker, performance notes): [hybrid.md](hybrid.md)

Kantra auto-switches to hybrid when non-Java languages are detected and you have not forced containerless. Override with `--run-local=true` or `--run-local=false` as needed.

```sh
# Containerless — Java + builtin on the host
kantra analyze --input=/path/to/app --output=/path/to/out --overwrite --target cloud-readiness

# Hybrid — providers in containers
kantra analyze --input=/path/to/app --output=/path/to/out --run-local=false --overwrite
```

Use `kantra analyze --help` for the full flag list. Analyzer internals: [analyzer.md](analyzer.md).

## Language providers

Providers implement language-specific rule conditions (LSP-based). Kantra picks providers from language detection (Alizer) unless you pass `--provider`.

List what Kantra supports:

```sh
kantra provider list
```

| Provider | Containerless (default) | Hybrid (`--run-local=false`) |
|----------|-------------------------|------------------------------|
| `java` | Yes | Yes |
| `golang` | No | Yes |
| `python` | No | Yes |
| `nodejs` | No | Yes |
| `csharp` | No | Yes |

- Restrict languages: `--provider java --provider python` (repeatable).
- Custom or remote providers: `--override-provider-settings=<config-file>` — see [hybrid.md](hybrid.md) and `kantra analyze --help`.
- See detected languages without analyzing: `kantra analyze --input=/path/to/app --list-languages`.

Default rulesets ship for **java**, **nodejs**, and **csharp**. If analysis finds a provider with no bundled rules and you did not pass `--rules`, Kantra exits with an error asking you to supply rules.

## Rules and filtering

### Default rulesets

Analysis loads curated rules from [konveyor/rulesets](https://github.com/konveyor/rulesets/) unless you disable them with `--enable-default-rulesets=false`.

### Scoping analysis

Filter which rules run:

| Flag | Purpose |
|------|---------|
| `--target` / `-t` | Migration path or goal (e.g. `cloud-readiness`, `quarkus`). Repeat for multiple targets. |
| `--source` / `-s` | Source stack (e.g. `eap7`, `springboot`). Repeat for multiple sources. |
| `--label-selector` / `-l` | Label expression (see [analyzer-lsp labels](https://github.com/konveyor/analyzer-lsp/blob/main/docs/labels.md)) |
| `--rules` | Extra rule file or directory (repeatable) |
| `--incident-selector` | Filter incidents in the report after analysis |

Discover available labels from bundled rules:

```sh
kantra rules list-targets
kantra rules list-sources
```

Examples:

```sh
kantra analyze --input ~/app --output ~/report --target cloud-readiness --overwrite
kantra analyze --input ~/app --output ~/report --source eap7 --target quarkus --mode source-only --overwrite
```

Rule writing and label conventions: [rules-quickstart.md](rules-quickstart.md). Test rule YAML with `kantra rules test`: [testrunner.md](testrunner.md).

### Custom rules and `--target`

When you pass `--rules`, those files are merged into the run. Rules are matched by labels:

- Built-in rules use `konveyor.io/target=<name>` and `konveyor.io/source=<name>` labels.
- If you pass `--target` (or `--source`) and a custom rule **does not** include the corresponding target (or source) label, that rule will **not** run.
- Add the right labels to your rule YAML and pass the same value on the CLI, for example:

```yaml
labels:
  - konveyor.io/target=my-migration
```

```sh
kantra analyze --input ~/app --output ~/report --rules ./my-rules --target my-migration --overwrite
```

With only custom rules and external providers, you may need `--enable-default-rulesets=false` and must supply `--rules`; see `kantra analyze --help`.

### Analysis depth

| Flag | Values / notes |
|------|----------------|
| `--mode` | `full` (default): source + dependencies where supported. `source-only`: application source only. |
| `--analyze-known-libraries` | Also analyze known open-source libraries (meaningful with `--mode full`) |
| `--no-dependency-rules` | Turn off dependency-related rules |
| `--dependency-folders` / `-d` | Extra dependency directories |

Full dependency analysis is implemented for the **Java** provider today.

### Hub profiles

Reuse team-wide settings (targets, label selectors, scope) from a profile file or synced Hub config:

```sh
kantra analyze --input ~/app --output ~/report --profile-dir ~/.kantra/profiles
```

Details: [profiles.md](profiles.md). Hub login/sync: `kantra config --help`.

## Output

Required flags for a normal run: `--input` and `--output`. Use `--overwrite` to replace an existing output directory.

After analysis, the output directory typically contains:

| Artifact | Description |
|----------|-------------|
| `static-report/` | HTML report (unless `--skip-static-report`) |
| `output.yaml` | Violations / issues |
| `dependencies.yaml` | Dependency graph (when applicable) |
| `analysis.log`, `dependency.log` | Logs |

Use `--json-output` for JSON analysis and dependency output. `--bulk` combines static reports when running multiple analyses in one session.

Walkthrough with sample paths: [examples.md](examples.md).

## Other commands

| Command | Purpose |
|---------|---------|
| `kantra transform openrewrite` | Run OpenRewrite recipes on Java (container; needs Podman/Docker) |
| `kantra rules` | List labels, run YAML rule tests |
| `kantra provider` | List analysis providers |
| `kantra config` | Konveyor Hub profiles |
| `kantra discover` / `kantra generate` | Asset-generation workflows — [examples.md](examples.md) |
| `kantra version` | CLI and component versions |

Global flags on any command: `--log-level`, `--no-cleanup` (see `kantra --help`).

## Related documentation

| Topic | Doc |
|-------|-----|
| Install & quick start | [README](../README.md) |
| Step-by-step analyze/transform examples | [examples.md](examples.md) |
| Containerless setup | [containerless.md](containerless.md) |
| Hybrid mode | [hybrid.md](hybrid.md) |
| Writing rules | [rules-quickstart.md](rules-quickstart.md) |
| Rule YAML tests | [testrunner.md](testrunner.md) |
| Analysis profiles / Hub | [profiles.md](profiles.md) |
| analyzer-lsp integration | [analyzer.md](analyzer.md) |
| Contributing / building | [developer.md](developer.md) |
