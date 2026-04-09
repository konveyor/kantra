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

1. **Releases:** [GitHub releases](https://github.com/konveyor/kantra/releases) — unzip and add `kantra` to your `PATH`.














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
- **Filtering** — `--target`, `--source`, `--label-selector`; list labels with `--list-targets` / `--list-sources`.

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
| `kantra test` | Run YAML rule tests |
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
| Provider options | [docs/usage.md](docs/usage.md) |
| Hack scripts / asset generation | [hack/README.md](hack/README.md) |

---

## Code of conduct

[Konveyor community Code of Conduct](https://github.com/konveyor/community/blob/main/CODE_OF_CONDUCT.md).
