# Kantra

Kantra is a CLI that unifies analysis and transformation capabilities of Konveyor.

## Installation

The easiest way to install Kantra is to get it via the container image.

1. To download latest container image using _podman_, follow instructions for your operating system:

  * For Linux, run:
  
    ```sh
    podman cp $(podman create --name kantra-download quay.io/konveyor/kantra:latest):/usr/local/bin/kantra . && podman rm kantra-download
    ```

  * For MacOS
  
    Prior to starting your podman machine, run:
  
    ```sh
    ulimit -n unlimited
    ```
    > This must be run after each podman machine reboot.
  
    Init your _podman_ machine :
  
    ```sh
    podman machine init <vm_name> -v $HOME:$HOME -v /private/tmp:/private/tmp -v /var/folders/:/var/folders/
    ```
  
    Increase podman resources:
  
    ```sh
    podman machine set <vm_name> --cpus 4 --memory 4096
    ```
  
    Ensure that we use the connection to the VM `<vm_name>` we created earlier by default:
    
    ```sh
    podman system connection default <vm_name>
    ```
  
    Finally, run:
  
    ```sh
    podman pull quay.io/konveyor/kantra:latest && podman run --name kantra-download quay.io/konveyor/kantra:latest 1> /dev/null 2> /dev/null && podman cp kantra-download:/usr/local/bin/darwin-kantra kantra && podman rm kantra-download
    ```
  
  * For Windows, run: 

    ```sh
    podman pull quay.io/konveyor/kantra:latest && podman run --name kantra-download quay.io/konveyor/kantra:latest 1> /dev/null 2> /dev/null && podman cp kantra-download:/usr/local/bin/windows-kantra kantra && podman rm kantra-download
    ```

2. The above will copy the binary into your current directory. Move it to PATH for system-wide use:

    ```sh
    sudo mv ./kantra /usr/local/bin/
    ```

3. To confirm Kantra is installed, run:

    ```sh
    kantra --help
    ```

    This should display the help message.

## Usage

Kantra has three subcommands:

1. _analyze_: This subcommand allows running source code analysis on input source code or a binary.

2. _transform_: This subcommand allows - converting XML rules to YAML, and running OpenRewrite recipes on source code.

3. _test_: This subcommand allows testing YAML rules.

### Analyze

_analyze_ subcommand allows running source code and binary analysis using [analyzer-lsp](https://github.com/konveyor/analyzer-lsp)

To run analysis on application source code, run:

```sh
kantra analyze --input=<path/to/source/code> --output=<path/to/output/dir>
```

_--input_ must point to a source code directory or a binary file, _--output_ must point to a directory to contain analysis results. 

All flags:

```sh
Flags:
      --analyze-known-libraries   analyze known open-source libraries
  -h, --help                      help for analyze
  -i, --input string              path to application source code or a binary
      --json-output               create analysis and dependency output as json
      --list-sources              list rules for available migration sources
      --list-targets              list rules for available migration targets
  -m, --mode string               analysis mode. Must be one of 'full' or 'source-only' (default "full")
  -o, --output string             path to the directory for analysis output
      --rules stringArray         filename or directory containing rule files. Use multiple times for additional rules: --rules <rule1> --rules <rule2> ...
      --skip-static-report        do not generate static report
  -s, --source stringArray        source technology to consider for analysis. Use multiple times for additional sources: --source <source1> --source <source2> ...
  -t, --target stringArray        target technology to consider for analysis. Use multiple times for additional targets: --target <target1> --target <target2> ...
```

### Transform

Transform has two subcommands:

1. _openrewrite_: This subcommand allows running one or more available OpenRewrite recipes on input source code.  

2. _rules_: This sucommand allows converting Windup XML rules into the analyzer-lsp YAML format.

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

## References 

- [Example usage scenarios](./docs/examples.md)
- [Test runner for YAML rules](./docs/testrunner.md)

## Code of Conduct

Refer to Konveyor's Code of Conduct [here](https://github.com/konveyor/community/blob/main/CODE_OF_CONDUCT.md).
