![Test](https://github.com/konveyor/kantra/actions/workflows/nightly.yaml/badge.svg?branch=main)

# Kantra

Kantra is a CLI for [Konveyor](https://konveyor.io/) analysis and transformation.

---

## Prerequisites

**Analysis Containerless mode**  — prerequisites and layout are documented in **[docs/containerless.md](docs/containerless.md)**

**Analysis Hybrid mode** (external language providers run in containers; the analyzer runs on the host.) and **Transform**  —

- **Podman 4+** or **Docker** (Engine 24+ / Desktop 4+). Kantra defaults to `podman`; override with `export CONTAINER_TOOL=/path/to/docker`.

---

## Install

### From GitHub releases (recommended)

Download the zip for your OS and CPU from [GitHub releases](https://github.com/konveyor/kantra/releases):

| Platform | Asset |
|----------|-------|
| Linux | `kantra.linux.amd64.zip`, `kantra.linux.arm64.zip` |
| macOS | `kantra.darwin.amd64.zip`, `kantra.darwin.arm64.zip` |
| Windows | `kantra.windows.amd64.zip`, `kantra.windows.arm64.zip` |

Each archive contains the CLI plus bundled assets (default rulesets, JDT language server, static report template, and Java/Maven helper files).

1. Unzip the archive into the default config directory (or another directory you will keep):

**Linux**

```sh
mkdir -p ~/.kantra
unzip kantra.linux.amd64.zip -d ~/.kantra   # or kantra.linux.arm64.zip
```

**macOS**

```sh
mkdir -p ~/.kantra
unzip kantra.darwin.amd64.zip -d ~/.kantra   # or kantra.darwin.arm64.zip
```

**Windows** (PowerShell)

```powershell
New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\.kantra"
Expand-Archive -Path kantra.windows.amd64.zip -DestinationPath "$env:USERPROFILE\.kantra"
# or kantra.windows.arm64.zip
```

You can also use **Extract All** in File Explorer and choose `%USERPROFILE%\.kantra` (for example `C:\Users\You\.kantra`).

1. Put the CLI on your `PATH`:

| Platform | Binary in the zip | Example |
|----------|-------------------|---------|
| Linux | `kantra` | `sudo mv ~/.kantra/kantra /usr/local/bin/` |
| macOS | `darwin-kantra` | `sudo mv ~/.kantra/darwin-kantra /usr/local/bin/kantra` |
| Windows | `windows-kantra.exe` | add to `PATH` as `kantra.exe` |

1. Verify: `kantra version`

Kantra resolves bundled assets in this order: `KANTRA_DIR` (if set) → current directory (when it contains `rulesets/`, `jdtls/`, and `static-report/`) → the default config directory: `$HOME/.kantra` on macOS, `%USERPROFILE%\.kantra` on Windows, and on Linux `$XDG_CONFIG_HOME/.kantra` when set or otherwise `$HOME/.kantra`. Containerless analysis requires those assets; hybrid and transform need the CLI and a container runtime (see [Prerequisites](#prerequisites)).

### Build from source (development)

Requires Go 1.25+.

```sh
git clone https://github.com/konveyor/kantra.git
cd kantra
go build -o kantra .
```

`go build` produces only the CLI — not the bundled rulesets, JDT LS, or report assets. Extract a [release zip](#from-github-releases-recommended) into the default config directory (for example `~/.kantra` on macOS/Linux or `%USERPROFILE%\.kantra` on Windows), or point `KANTRA_DIR` at that directory. See [docs/developer.md](docs/developer.md) for container-based development workflows.

## Quick start: analyze

**Containerless (default)** — Java + builtin on the host:

```sh
kantra analyze --input=/path/to/app --output=/path/to/out --overwrite --target cloud-readiness
```

**Hybrid** — language providers in containers, engine on the host (needed for Go, Python, Node.js, C#, or Java in a container):

```sh
kantra analyze --input=/path/to/app --output=/path/to/out --run-local=false --overwrite
```

- **`--mode`** — `full` (default, source + deps where supported) or `source-only`.  
- **Filtering** — `--target`, `--source`, `--label-selector`; list labels with `kantra rules list-targets` / `kantra rules list-sources`.

Deeper detail: **[docs/containerless.md](docs/containerless.md)**, **[docs/hybrid.md](docs/hybrid.md)**.

## Quick start: transform

Run [OpenRewrite](https://docs.openrewrite.org/) recipes on Java source (runs in a container; requires Podman/Docker):

```sh
kantra transform openrewrite --list-targets
kantra transform openrewrite --input=/path/to/app --target=<recipe-name>
```

---

## Other commands

| Command | Purpose |
|--------|---------|
| `kantra config` | Login / sync / list Konveyor Hub profiles (`kantra config --help`) |
| `kantra rules` | List rule labels and run YAML rule tests (`kantra rules --help`) |
| `kantra discover` / `kantra generate` | Asset-generation workflows; see **[docs/examples.md](docs/examples.md)** |

Use `kantra <command> --help` for flags.

---

## Documentation

| Topic | Doc |
|-------|-----|
| Examples | [docs/examples.md](docs/examples.md) |
| Hybrid / containerless analysis | [docs/hybrid.md](docs/hybrid.md), [docs/containerless.md](docs/containerless.md) |
| Rule YAML tests | [docs/testrunner.md](docs/testrunner.md) |
| Writing rules | [docs/rules-quickstart.md](docs/rules-quickstart.md) |
| Usage (modes, providers, rules, output) | [docs/usage.md](docs/usage.md) |
| Hack scripts / asset generation | [hack/README.md](hack/README.md) |

---

## Code of conduct

[Konveyor community Code of Conduct](https://github.com/konveyor/community/blob/main/CODE_OF_CONDUCT.md).
