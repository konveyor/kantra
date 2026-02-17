### Running kantra

#### Environment variables

- `CONTAINER_TOOL`: path to your container runtime (podman or docker)
- `RUNNER_IMG`: the tag of the kantra image to invoke
- `KANTRA_DIR`: directory used for rulesets, Java tooling (jdtls), and static-report assets. If set, this overrides the default resolution (see below).

**Kantra directory resolution (analyze, dump-rules, etc.)**

The “kantra directory” is chosen in this order of priority:

1. **`KANTRA_DIR`** — if the environment variable is set, that path is used.
2. **Current working directory** — if it contains the subdirectories `rulesets`, `jdtls`, and `static-report`, it is used.
3. **Config directory** — otherwise:
   - Linux: `$XDG_CONFIG_HOME/.kantra` if `XDG_CONFIG_HOME` is set, else `$HOME/.kantra`
   - macOS / Windows: `$HOME/.kantra` or `%USERPROFILE%\.kantra`

Setting `KANTRA_DIR` is useful when you run kantra from a different working directory (e.g. in scripts or when the process is started with a different `cwd`).

#### example:

`podman build -f Dockerfile -t kantra:dev`

`RUNNER_IMG=kantra:dev CONTAINER_TOOL=/usr/local/bin/podman go run main.go analyze --input=<path-to-src-application> --output=./output`

#### Helpful flags:

- To increase logs for debugging, you can set `--log-level` (default is 5)
- ie: `--log-level=7`
