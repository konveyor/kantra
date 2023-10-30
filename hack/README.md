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

