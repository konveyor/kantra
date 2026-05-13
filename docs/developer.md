### Running kantra

#### Environment variables

- `CONTAINER_TOOL`: path to your container runtime (podman or docker)
- `RUNNER_IMG`: the tag of the kantra image to invoke
- `ASSETS_PATH`: directory used for rulesets, Java tooling (jdtls), and static-report assets (preferred over `KANTRA_DIR`).
- `KANTRA_DIR`: legacy alias for `ASSETS_PATH` (upstream compatibility).

**Assets directory resolution (`kantra analyze`, `dump-rules`, etc.)**

The assets root is chosen in this order of priority:

1. **`--assets-path`** — CLI flag on `analyze` (highest priority).
2. **`ASSETS_PATH`** — environment variable.
3. **`KANTRA_DIR`** — legacy environment variable.
4. **Current working directory** — if it contains the subdirectories `rulesets`, `jdtls`, and `static-report`.
5. **Config directory** — otherwise:
   - Linux: `$XDG_CONFIG_HOME/.kantra` if `XDG_CONFIG_HOME` is set, else `$HOME/.kantra`
   - macOS / Windows: `$HOME/.kantra` or `%USERPROFILE%\.kantra`

Only the root path is configurable; subdirectory names (`rulesets`, `jdtls`, `static-report`, etc.) are fixed.

Setting `ASSETS_PATH` or `--assets-path` is useful when you run from a different working directory or use a non-default install location (e.g. downstream MTA packaging).

#### example:

`podman build -f Dockerfile -t kantra:dev`

`RUNNER_IMG=kantra:dev CONTAINER_TOOL=/usr/local/bin/podman go run main.go analyze --input=<path-to-src-application> --output=./output`

#### Helpful flags:

- To increase logs for debugging, you can set `--log-level` (default is 5)
- ie: `--log-level=7`

- Set `-no-progress` to see additional kantra logs
