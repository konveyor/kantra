# Scripts to help with various 'hacking' needs

## Running on Mac (darwin)
* `darwin_get_latest_kantra_cli.sh`:  Run to fetch latest image and extract kantra binary for Mac

* `darwin_restart_podman_machine.sh`:  Help to create/restart a podman machine VM to use with Kantra on Mac
	* Can customize like: `CPUS=8 MEM=16384 ./darwin_restart_podman_machine.sh`

## General run

* `sample_jakartaee_duke_analyze.sh`:  Clone and analyze a jakartaee sample

## Gather analysis traces 

Full traces of the analysis process can be collected via Jaeger. 

First, run the Jaeger container that will collect traces from analysis:

```sh
podman run -d --net=host --name jaeger -e COLLECTOR_ZIPKIN_HOST_PORT=:9411 jaegertracing/all-in-one:1.23
```

> Note that we are running Jaeger in `host` network so that analyzer container can communicate with it later. There are several services running in the container, it could happen that the ports they use are pre-occupied on your host. In that case, you will need to free them up manually.

The Jaeger collector listens on `14268` while the UI listens on `16686` in the container.

Now, we will run analysis by enabling the tracer that will send traces to our Jaeger collector. 

To do that, pass `--jaeger-endpoint` flag with the value being the collector endpoint:

```sh
kantra analyze <YOUR_OPTIONS_HERE> --jaeger-enpoint 'http://172.17.0.1:14268/api/traces'
```

> Note that `172.17.0.1` is the IP address using which a Podman container can communicate with the host on Linux. It might be different for your system. Alternatively, you can create a network, set it as default and create your Jaeger instance in it to access it via a name instead of IP.

When analysis finishes, download the traces from Jaeger:

```sh
curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp
```

To view the results locally, open [http://localhost:16686/](http://localhost:16686/) in your browser

## Setup dev environment for asset-generation

The script `hack/asset-generation-dev-setup.sh` checks your environment and
create a KinD cluster with Korifi.

If the `kind` command is not available, the script will install it for you automatically.

### Prerequisite
* go >= 1.22.9
* kubectl
* Docker (rootless) >= 20.10 or Podman >= 3.0

### Usage

```bash
./asset-generation-dev-setup.sh --help
Usage: ./asset-generation-dev-setup.sh [--docker | --podman] [--version] [--cleanup] [--help]

Flags:
  --docker        Use Docker as the container runtime
  --podman        Use Podman as the container runtime
  --cleanup       Delete kind cluster and Korifi installation
  --version       Show the version of the script
  --help          Show this help message

Note:
  If no runtime is specified, Podman will be used by default.
```

#### Example

- **Start a KinD cluster using Docker**

`./asset-generation-dev-setup.sh --docker`

- **Clean up the environment (delete cluster and Korifi):**
`./asset-generation-dev-setup.sh --cleanup`

### Troubleshooting
* `cannot expose privileged port 80`

If during the KinD cluster creation you get an error like this 

```bash
Command Output: 601810129f0585192f5ef3cacbb9c3d330a3a2297e8878c3a87504830e8a5377
docker: Error response from daemon: failed to set up container networking: driver failed programming external connectivity on endpoint korifi-control-plane (0935aba6417b2c48ab62064e9b4cfe633d83334310f0ed53baa3008ffd5c2c6a): error while calling RootlessKit PortManager.AddPort(): cannot expose privileged port 80, you can add 'net.ipv4.ip_unprivileged_port_start=80' to /etc/sysctl.conf (currently 1024), or set CAP_NET_BIND_SERVICE on rootlesskit binary, or choose a larger port number (>= 1024): listen tcp4 0.0.0.0:80: bind: permission denied
```

As suggested append this line `net.ipv4.ip_unprivileged_port_start=80` into
`/etc/sysctl.conf`. For your convenience you can execute:

```bash
sudo grep -q '^net.ipv4.ip_unprivileged_port_start=80' /etc/sysctl.conf || echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee -a /etc/sysctl.conf
```
Finally, reload kernel parameters executing `sysctl -p`

* `Cannot connect to the Docker daemon at unix:///home/<user>>/.docker/run/docker.sock`
First check the status of the Docker daemon with:
```bash
systemctl --user status docker
```
If it's not running, you can start it using:
```bash
systemctl --user start docker
```

You may also need to set the `DOCKER_HOST` environment variable:

```bash
export DOCKER_HOST=unix://$XDG_RUNTIME_DIR/docker.sock
```

💡 To make this change permanent, consider adding the export line to your shell
profile (~/.bashrc, ~/.zshrc, etc.).

* `fsnotify watcher: too many open files`

To fix it temporarily (until next reboot)

```bash
sudo sysctl -w fs.inotify.max_user_watches=1048576
```

To fix it permanently, you need to use sysctl to configure your kernel on boot.
Write the following line to a appropriately-named file under `/etc/sysctl.d/`, for example `/etc/sysctl.d/inotify.conf`:

```bash
fs.inotify.max_user_watches=1048576
```

* Pod `envoy-korifi` can't start

If the envoy-korifi pod isn't starting, you can begin troubleshooting by inspecting its logs. Adapt the following command to your specific pod name:

`oc logs -n korifi-gateway envoy-korifi-fvpcd -c envoy`

If you see an error like this:

```bash
[...][assert] [source/common/filesystem/inotify/watcher_impl.cc:23] assert failure: inotify_fd_ >= 0. Details: Consider increasing value of user.max_inotify_watches via sysctl
```
It indicates that the system has hit the limit for inotify watches—a
kernel-level resource needed for file system monitoring.

To fix it temporarily (until next reboot)

```bash
sudo sysctl -w fs.inotify.max_user_watches=1048576
sudo sysctl -w fs.inotify.max_user_instances=8192
```

To persist these settings across reboots, create a sysctl configuration file.

Append the following lines to a appropriately-named file under `/etc/sysctl.d/`, for example `/etc/sysctl.d/inotify.conf`:

```bash
fs.inotify.max_user_watches=1048576
fs.inotify.max_user_instances=8192
```