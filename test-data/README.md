# Analysis test data

Subdirectories contain kantra analysis testcases files. The following structure is expected:

```
cmd (kantra analyze command with options specific for given test case)

output.yaml (expected analysis output yaml file)

dependencies.yaml (optional, expected dependencies files created by analysis)

.gitignore (optional, should cover output directory and temporary files created by analysis)
```

A script `hack/test_analysis.sh` runs the kantra command and checks outputs. Exit code 0 is success, other failure. The script changes directory to the provided test-dir, so all paths there should be relative.

## Tests execution

Example usage from kantra project directory:

```
$ ./hack/test_analysis.sh test-data/<analysis-test-dir>

```

Or from actual test directory
```
$ ../../hack/test_analysis.sh .
```
