![Test](https://github.com/konveyor/kantra/actions/workflows/nightly.yaml/badge.svg?branch=main)

# Kantra

Kantra is a CLI that unifies analysis and transformation capabilities of Konveyor. It is available for Linux, Mac and Windows.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Setup (For Mac and Windows Only)](#setup-for-mac-and-windows-only)
- [Usage](#usage)
  - [Analyze an application](#analyze)
  - [Transform an application or XML rules](#transform)
  - [Test YAML rules](#test)
- [References](#references)
- [Code of conduct](#code-of-conduct)

## Prerequisites

_Podman 4+_ is required to run kantra. By default, it is configured to use the podman executable available on the host. 

Although kantra is primarily tested with podman, _Docker Engine 24+_ or _Docker Desktop 4+_ can be used as an alternative. To use docker, set the environment variable `CONTAINER_TOOL` pointing to the docker executable's path:

```sh
export CONTAINER_TOOL=/usr/bin/docker
```

## Installation

To install kantra, download the executable for your platform and add it to the path.  

Note: For Mac you might get a "Apple could not verify" error message. If you do you can run `xattr -d com.apple.quarantine kantra` to have Apple trust the `kantra` binary.

### Downloading stable release

Go to the [release page](https://github.com/konveyor/kantra/releases) and download the zip file containing a binary for your platform and architecture. Unzip the archive and add the executable in it to the path. 

### Downloading the latest / particular version

The easiest way to get the latest (or a particular/older) executable is to get it from the respective container image.

Set the shell variable `kantra_version` to a particular version if you want to grab that version,
e.g., `kantra_version=v0.4.0`. 

#### Linux

Run:

```sh
${CONTAINER_TOOL:-podman} cp $(${CONTAINER_TOOL:-podman} create --name kantra-download quay.io/konveyor/kantra:${kantra_version:-latest}):/usr/local/bin/kantra . && ${CONTAINER_TOOL:-podman} rm kantra-download
```

#### Mac

When you are not using Docker for Desktop on your Mac (see above), 
you need to start a podman machine prior to running any podman commands (see [Setup for Mac](#mac-1))

Once a machine is started, run:

```sh
${CONTAINER_TOOL:-podman} cp $(${CONTAINER_TOOL:-podman} create --name kantra-download quay.io/konveyor/kantra:${kantra_version:-latest}):/usr/local/bin/darwin-kantra . && ${CONTAINER_TOOL:-podman} rm kantra-download
```

#### Windows

When you are not using Docker for Desktop on your Windows (see above),
you need to start a podman machine prior to running any podman commands (see [Setup for Windows](#windows-1))

Once a machine is started, run:

```sh
${CONTAINER_TOOL:-podman} cp $(${CONTAINER_TOOL:-podman} create --name kantra-download quay.io/konveyor/kantra:${kantra_version:-latest}):/usr/local/bin/windows-kantra . && ${CONTAINER_TOOL:-podman} rm kantra-download
```

> Ensure that you add the executable to the `PATH`.

## Setup (For Mac and Windows Only)

On Mac and Windows, a podman machine needs to be started prior to running any commands, unless you are using Docker (for Desktop):

##### Mac
      
Prior to starting your podman machine, run:
    
```sh
ulimit -n unlimited
```

Init your podman machine :

  * _Podman 4_:
        
    Podman 4 requires some host directories to be mounted within the VM:

    ```sh
    podman machine init <vm_name> -v $HOME:$HOME -v /private/tmp:/private/tmp -v /var/folders/:/var/folders/
    ```

  * _Podman 5_:

    Podman 5 mounts _$HOME_, _/private/tmp_ and _/var/folders_ directories by default, simply init the machine:

    ```sh
    podman machine init <vm_name>
    ```

> If the input and/or output directories you intend to use with kantra fall outside the tree of $HOME, /private/tmp and /var/folders directories, you should mount those directories in addition to the default.

Increase podman resources (minimum 4G memory is required):
    
```sh
podman machine set <vm_name> --cpus 4 --memory 4096
```

##### Windows

Init the machine:

```sh
podman machine init <vm_name>
```

## Usage

Kantra has five subcommands:

1. _analyze_: This subcommand allows running source code analysis on input source code or a binary.

2. _transform_: This subcommand allows either converting XML rules to YAML or running OpenRewrite recipes on source code.

3. _test_: This subcommand allows testing YAML rules.

4. _discover_: This subcommand allows to discover application and outputs a YAML representation of source platform resources.

5. _generate_: This subcommand allora to analyze the source plaftform and/or application and output a discovery manifest.

### Analyze

_analyze_ subcommand allows running source code and binary analysis using [analyzer-lsp](https://github.com/konveyor/analyzer-lsp)

To run analysis on application source code, run:

```sh
kantra analyze --input=<path/to/source/code> --output=<path/to/output/dir>
```

_--input_ must point to a source code directory or a binary file, _--output_ must point to a directory to contain analysis results. 

All flags:

```
Flags:
      --analyze-known-libraries          analyze known open-source libraries
      --bulk                             running multiple analyze commands in bulk will result to combined static report
      --context-lines int                number of lines of source code to include in the output for each incident (default 100)
  -d, --dependency-folders stringArray   directory for dependencies
      --enable-default-rulesets          run default rulesets with analysis (default true)
  -h, --help                             help for analyze
      --http-proxy string                HTTP proxy string URL
      --https-proxy string               HTTPS proxy string URL
      --incident-selector string         an expression to select incidents based on custom variables. ex: (!package=io.konveyor.demo.config-utils)
  -i, --input string                     path to application source code or a binary
      --jaeger-endpoint string           jaeger endpoint to collect traces
      --json-output                      create analysis and dependency output as json
  -l, --label-selector string            run rules based on specified label selector expression
      --list-sources                     list rules for available migration sources
      --list-targets                     list rules for available migration targets
      --maven-settings string            path to a custom maven settings file to use
  -m, --mode string                      analysis mode. Must be one of 'full' or 'source-only' (default "full")
      --no-proxy string                  proxy excluded URLs (relevant only with proxy)
  -o, --output string                    path to the directory for analysis output
      --overwrite                        overwrite output directory
      --rules stringArray                filename or directory containing rule files. Use multiple times for additional rules: --rules <rule1> --rules <rule2> ...
      --skip-static-report               do not generate static report
  -s, --source stringArray               source technology to consider for analysis. Use multiple times for additional sources: --source <source1> --source <source2> ...
  -t, --target stringArray               target technology to consider for analysis. Use multiple times for additional targets: --target <target1> --target <target2> ...
```

#### Analyze multiple applications

By design, kantra supports single application analysis per kantra command execution. However, it is possible use ```--bulk``` option for executing multiple kantra analyze commands with different applications to get an output directory and static-report populated with all applications analysis reports.

Example:
```sh
kantra analyze --bulk --input=<path/to/source/A> --output=<path/to/output/ABC>
kantra analyze --bulk --input=<path/to/source/B> --output=<path/to/output/ABC>
kantra analyze --bulk --input=<path/to/source/C> --output=<path/to/output/ABC>
```

### Transform

Transform has two subcommands:

1. _openrewrite_: This subcommand allows running one or more available OpenRewrite recipes on input source code.  

2. _rules_: This subcommand allows converting Windup XML rules into the analyzer-lsp YAML format.

#### OpenRewrite

_openrewrite_ subcommand allows running [OpenRewrite](https://docs.openrewrite.org/) recipes on source code.

To transform applications using OpenRewrite, run:

```sh
kantra transform openrewrite --input=<path/to/source/code> --target=<exactly_one_target_from_the_list>
```

The value of _--target_ option must be one of the available OpenRewrite recipes. To list all available recipes, run: 

```sh
kantra transform --list-targets
```

All flags:

```sh
Flags:
  -g, --goal string             target goal (default "dryRun")
  -h, --help                    help for openrewrite
  -i, --input string            path to application source code directory
  -l, --list-targets            list all available OpenRewrite recipes
  -s, --maven-settings string   path to a custom maven settings file to use
  -t, --target string           target openrewrite recipe to use. Run --list-targets to get a list of packaged recipes.
```

#### Rules

_rules_ subcommand allows converting Windup XML rules to analyzer-lsp YAML rules using [windup-shim](https://github.com/konveyor/windup-shim)

To convert Windup XML rules to the analyzer-lsp YAML format, run:

```sh
kantra transform rules --input=<path/to/xmlrules> --output=<path/to/output/dir>
```

_--input_ flag should point to a file or a directory containing XML rules, _--output_ should point to an output directory for YAML rules.

All flags:

```sh
Flags:
  -h, --help                help for rules
  -i, --input stringArray   path to XML rule file(s) or directory
  -o, --output string       path to output directory
```

### Test

_test_ subcommand allows running tests on YAML rules written for analyzer-lsp. 

The input to test runner will be one or more test files and / or directories containing tests written in YAML.

```sh
kantra test /path/to/a/single/tests/file.test.yaml
```

The output of tests is printed on the console.

See different ways to run the test command in the [test runner doc](./docs/testrunner.md#running-tests)

### Asset Generation

Asset generation consist of two subcommands _discover_ and _generate_.

#### Discover
Discover application outputs a YAML representation of source platform resources.

To run a discover, run:

`kantra discover --input=<path/to/yaml/manifest>`

_--input_ must point to a yaml manifest file.


All flags:

```sh
Flags:
  -h, --help             help for discover
      --list-platforms   List available supported discovery platform.
```

#### Generate

Analyze the source platform and/or application and output discovery manifest.


To generate a discovery manifest, run:

`kantra generate helm --input=<path/to/discover/manifest> --chart-dir=<path/to/helmchart>`

All flags

```sh
Flags:
  -h, --help   help for generate
```
_generate_ subcommand has a _helm_ subcommand that generates the helm template manifest.

All flags:

```sh
Flags:
      --chart-dir string    Directory to the Helm chart to use for chart generation.
  -h, --help                help for helm
      --input string        Specifies the discover manifest file
      --non-k8s-only        Render only the non-Kubernetes templates located in the files/konveyor directory of the chart
      --output-dir string   Directory to save the generated Helm chart. Defaults to stdout
      --set stringArray     Set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)
```

## References 

- [Example usage scenarios](./docs/examples.md)
- [Using provider options](./docs/usage.md)
- [Test runner for YAML rules](./docs/testrunner.md)

## Code of Conduct

Refer to Konveyor's Code of Conduct [here](https://github.com/konveyor/community/blob/main/CODE_OF_CONDUCT.md).
