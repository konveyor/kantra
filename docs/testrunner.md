# Test Runner for YAML rules

Via the _test_ subcommand, _kantra_ exposes a test runner. 

It allows testing YAML rules written for [analyzer-lsp](https://github.com/konveyor/analyzer-lsp).

The input to the test runner are tests written in YAML, the output of the test runner is a report.

## Usage

This section covers:

1. [Writing tests](#writing-tests)
2. [Running tests](#running-tests)
3. [Understanding output](#test-output)

### Writing tests

Tests for a rules file are written in a YAML file with names ending in `.test.yaml` suffix.

A tests file contains three fields _rulesPath_, _providers_ and _tests_ at the top level: 

```yaml
rulesPath: "/optional/path/to/rules/file"
providers:
  - name: "go"
    dataPath: "/path/to/data/for/this/provider"
tests:
  - ruleID: "rule-id-for-this-test"
    testCases:
      - name: "test-case-name"
      [...]
```

* _rulesPath_: Relative path to a file containing rules these tests are applicable to 
* _providers_: List of configs, each containing configuration for a specific provider to be used when running tests
* _tests_: List of tests to run, each containing test definition for a specific rule in the associated rules file

> Note that _rulesPath_ is optional. If it is not specified, the runner will look for a file in the same directory with the same name as the tests file except the _.tests.yaml_ suffix in the name.

#### Defining providers

The field _providers_ defines a list of configs, each specific to a provider:

```yaml
providers:
  - name: <name_of_the_provider>
    dataPath: <path_to_test_data>
tests:
  [...]
```

_name_ is the name of the provider to which the config applies to, and _dataPath_ is the relative path to the test data to be used when testing rules for that provider.

> Note that _dataPath_ must be relative to the directory in which tests file exists.

If all tests under a _ruleset_ share values of _providers_ field (e.g. they use common data directory in all tests for a given provider), this config can also be defined at ruleset level under a special file `testing-config.yaml`. In that case, config present in this file will apply to all tests in that directory. A more specific config for a certain file can still be defined in the tests file. In that case, values in the tests file will take precedance over values at the _ruleset_ level.

See an example of ruleset level config in [../pkg/testing/examples/ruleset/testing-config.yaml](../pkg/testing/examples/ruleset/testing-config.yaml).

> Note that a config for every providers present in the rules file _must_ be defined.

#### Defining tests

The field _tests_ defines a list of tests, each specific to a rule in the rules file:

```yaml
providers:
  [...]
tests:
  - ruleID: test-00
    testCases:
      - name: test-tc-00
        analysisParams:
          depLabelSelector: "!konveyor.io/source=open-source"
          mode: "full"
        hasIncidents:
            exactly: 10
            messageMatches: "test"
            codeSnipMatches: "test"
      - name: test-tc-01
        analysisParams:
          mode: "source-only"
        hasTags:
          - "test"
        hasIncidents:
          locations:
          - lineNumber: 10
            fileURI: file://test
            messageMatches: "message"     
```
###### Test

| Field     | Type        | Required | Description                                    |
| --------- | ---------   | -------- | ---------------------------------------------  | 
| ruleID    | string      | Yes      | ID of the rule this test applies to            |
| testCases | []TestCase  | Yes      | List of test cases (See [TestCase](#testcase)) |
 
###### TestCase

| Field          | Type            | Required | Description                                                                                            |
| ---------      | ---------       | -------- | ------------------------------------------------------------------------------------------------------ | 
| name           | string          | Yes      | Unique name for the test case, can be used to filter test case.                                        |
| analysisParams | AnalysisParams  | Yes      | Analysis parameters to use when running this test case (See [AnalysisParams](#analysisparams))         |
| hasIncidents   | HasIncidents    | No       | Passing criteria that compares produced incidents (See [HasIncidents](#hasincidents))                  | 
| hasTags        | []string        | No       | Passing criteria that compares produced tags, passes test case when all tags are present in the output |
| isUnmatched    | bool            | No       | Passes the test case when rule is NOT matched                                                          |

###### AnalysisParams

| Field            | Type    | Required | Description                                                                          |
| ---------------- | ------- | -------- | ------------------------------------------------------------------------------------ | 
| depLabelSelector | string  | No       | Dependency label selector expression to pass as --dep-label-selector to the analyzer |
| mode             | string  | No       | Analysis mode, one of - _source-only_ or _full_                                      |

###### HasIncidents

_HasIncidents_ defines a criteria for passing the test case. It provides two ways to define a criteria, either one of the two can be defined in a test case:

1. _Count based_: This criteria is based on count of incidents. It can be defined using following fields under _hasIncidents_:

      | Field           | Type        | Required | Description                                                                             |
      | --------------- | ----        | -------- | --------------------------------------------------------------------------------------- | 
      | exactly         | int         | Yes      | Produced incidents should be exactly equal to this number for test case to pass         |
      | atLeast         | int         | Yes      | Produced incidents should be greater than or equal to this number for test case to pass | 
      | atMost          | int         | Yes      | Produced incidents should be less than or equal to this number for test case to pass    |
      | messageMatches  | int         | No       | In all incidents, message should match this pattern for test case to pass               |
      | codeSnipMatches | int         | No       | In all incidents, code snippet should match this pattern for test case to pass          |
      
      > Only one of _exactly_, _atLeast_, or _atMost_ can be defined at a time

2. _Location based_: This criteria is based on location of each incident. It can be defined using following fields under _hasIncidents_:

      | Field           | Type        | Required | Description                                                                         |
      | --------------- | ----        | -------- | ----------------------------------------------------------------------------------- | 
      | locations       | []Location  | No       | Passing criteria that is based on location of each incident rather than just count  |
      
      Each _Location_ has following fields:
      
      | Field           | Type        | Required | Description                                                         |
      | --------------- | ----        | -------- | ------------------------------------------------------------------- | 
      | fileURI         | string      | Yes      | An incident must be found in this file for test case to pass        | 
      | lineNumber      | string      | Yes      | An incident must be found on this line number for test case to pass |
      | messageMatches  | int         | No       | Message should match this pattern for test case to pass             |
      | codeSnipMatches | int         | No       | Code snippet should match this pattern for test case to pass        |

### Running tests

To run tests in a single file:

```yaml
kantra test /path/to/a/single/tests/file.test.yaml
```

To run tests in a ruleset:

```yaml
kantra test /path/to/a/ruleset/directory/
```

To run tests in multiple different paths:

```yaml
kantra test /path/to/a/ruleset/directory/ /path/to/a/test/file.test.yaml
```

To run specific tests by rule IDs:

```yaml
kantra test /path/to/a/ruleset/directory/ -t "RULE_ID_1, RULE_ID_2"
```

_-t_ option allows specifying a list of rule IDs (separated by commas) to select specific tests.

A specific test case in a test can also be selected using the _-t_ option.

To run specific test cases in a test, each value in the comma separated list of  _-t_ becomes _<RULE_ID>#<TEST_CASE_NAME>_:

```yaml
kantra test /path/to/a/ruleset/directory/ -t RULE_ID_1#TEST_CASE_1
```

> Note that # is a reserved character used to seperate test case name in the filter. The name of the test case itself _must not_ contain #. 

### Test Output

When a test passes, the runner creates output that looks like:

```sh
- 156-java-rmi.windup.test.yaml 2/2 PASSED
 - java-rmi-00000               1/1 PASSED
 - java-rmi-00001               1/1 PASSED
------------------------------------------------------------
  Rules Summary:      2/2 (100.00%) PASSED
  Test Cases Summary: 2/2 (100.00%) PASSED
------------------------------------------------------------
```

The runner will clean up all temporary directories when all tests in a file pass.

If a test fails, the runner will create output that looks like: 

```sh
- 160-local-storage.windup.test.yaml 0/1 PASSED
 - local-storage-00001               0/1 PASSED
   - tc-1                            FAILED
     - expected at least 48 incidents, got 18
     - find debug data in /tmp/rules-test-242432604
------------------------------------------------------------
  Rules Summary:      0/1 (0.00%) PASSED
  Test Cases Summary: 0/1 (0.00%) PASSED
------------------------------------------------------------
```

In this case, the runner leaves the temporary directories behind for debugging. In the above example, the temporary directory is `/tmp/rules-test-242432604`.

Among other files, the important files needed for debugging in this directory are:

* _analysis.log_: This file contains the full log of analysis
* _output.yaml_: This file contains the output generated post analysis
* _provider\_settings.json_: This file contains the provider settings used for analysis
* _rules.yaml_: This file contains the rules used for analysis
* _reproducer.sh_: This file contains a command you can run directly on your system to reproduce the analysis as-is.

> In the temporary directory, there could be files generated by the providers including their own logs. Those files can be useful for debugging too.

### References

- OpenAPI schema for tests: [Tests schema](../test-schema.json)

- Example tests for a ruleset: [Ruleset tests](../pkg/testing/examples/ruleset/)

- Example tests for a rules file: [Rules file tests](../pkg/testing/examples/rules-file.test.yaml)
