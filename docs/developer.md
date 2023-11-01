### Running kantra

Two environment variables control the container runtime and the kantra image: `PODMAN_BIN` and `RUNNER_IMG`:
- `PODMAN_BIN`: path to your container runtime (podman or docker)
- `RUNNER_IMG`: the tag of the kantra image to invoke

#### example:

`podman build -f Dockerfile -t kantra:dev`

`RUNNER_IMG=kantra:dev PODMAN_BIN=/usr/local/bin/podman go run main.go analyze --input=<path-to-src-application> --output=./output`

#### Helpful flags:
- To increase logs for debugging, you can set `--log-level` (default is 5)
- ie: `--log-level=7`
