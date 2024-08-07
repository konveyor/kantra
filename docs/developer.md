### Running kantra

Two environment variables control the container runtime and the kantra image: `CONTAINER_TOOL` and `RUNNER_IMG`:
- `CONTAINER_TOOL`: path to your container runtime (podman or docker)
- `RUNNER_IMG`: the tag of the kantra image to invoke

#### example:

`podman build -f Dockerfile -t kantra:dev`

`RUNNER_IMG=kantra:dev CONTAINER_TOOL=/usr/local/bin/podman go run main.go analyze --input=<path-to-src-application> --output=./output`

#### Helpful flags:

- To increase logs for debugging, you can set `--log-level` (default is 5)
- ie: `--log-level=7`
