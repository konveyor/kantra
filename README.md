# Kantra

Kantra is an experimental CLI that unifies analysis and transformation capabilities of Konveyor.

## Installation

Easiest way to install Kantra is to get it via the container image. To download latest container image, run:

```sh
podman pull quay.io/konveyor/kantra:latest && podman run --name kantra-download quay.io/konveyor/kantra:latest 1> /dev/null 2> /dev/null && podman cp kantra-download:/usr/local/bin/kantra . && podman rm kantra-download
```

This will copy the binary into your current working directory. To make it available system-wide, run:

```sh
sudo mv ./kantra /usr/local/bin/
```

To confirm Kantra is installed, run:

```sh
kantra --help
```

This should display the help message.

## Usage

Kantra has two subcommands - `analyze` and `transform`:


```sh
A cli tool for analysis and transformation of applications

Usage:
  kantra [command]

Available Commands:
  analyze     Analyze application source code
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  transform   Transform application source code or windup XML rules

Flags:
  -h, --help            help for kantra
      --log-level int   log level (default 5)

Use "kantra [command] --help" for more information about a command.
```

### Analyze

Analyze allows running source code and binary analysis using analyzer-lsp.

To run analysis on application source code, run:

```sh
kantra analyze --input=<path/to/source/code> --output=<path/to/output/dir>
```

All flags:

```sh
Analyze application source code

Usage:
  kantra analyze [flags]

Flags:
      --analyze-known-libraries   analyze known open-source libraries
  -h, --help                      help for analyze
  -i, --input string              path to application source code or a binary
      --list-sources              list rules for available migration sources
      --list-targets              list rules for available migration targets
  -m, --mode string               analysis mode. Must be one of 'full' or 'source-only' (default "full")
  -o, --output string             path to the directory for analysis output
      --rules stringArray         filename or directory containing rule files
      --skip-static-report        do not generate static report
  -s, --source stringArray        source technology to consider for analysis
  -t, --target stringArray        target technology to consider for analysis

Global Flags:
      --log-level int   log level (default 5)
```

### Transform

Transform has two subcommands - `openrewrite` and `rules`.

#### OpenRewrite

`openrewrite` subcommand allows running OpenRewrite recipes on source code.


#### Rules

`rules` subcommand allows converting Windup XML rules to analyzer-lsp YAML rules.

## Code of Conduct
Refer to Konveyor's Code of Conduct [here](https://github.com/konveyor/community/blob/main/CODE_OF_CONDUCT.md).
