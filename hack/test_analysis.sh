#!/bin/bash
#
# Usage: run_analysis_test <test-data-directory>
#
set -e

if [ ! -d $1 ]; then
    echo "ERROR: Missing or invalid test-data directory."
    exit 1
fi
test_dir=$1
cd ${test_dir}

# Setup variables
RUNNER_IMG="${RUNNER_IMG:-quay.io/konveyor/kantra:latest}"
KANTRA_CMD="${KANTRA_CMD:-../../kantra}"
TEST_APPS_ROOT="${TEST_APPS_ROOT:-../../hack/tmp}"

# Run the analysis calling kantra
. ./cmd
kantra_exit=$?
if [ "$kantra_exit" != 0 ]; then
    echo "ERROR: kantra command execution failed."
    exit $kantra_exit
fi

# Check analysis result
expected_file=output.yaml
actual_file=output/output.yaml
function filter_and_sort_file() {
  yq -i e 'del(.[].skipped) | del(.[].unmatched)' $1
  yq -i e '.[]?.violations |= (. | to_entries | sort_by(.key) | from_entries)' $1
  yq -i e '.[]?.violations[]?.incidents |= sort_by(.uri)' $1
  yq -i e '.[] | (.tags // []) |= sort' $1
}
# Expected files should be started as copy of verified actual output.yaml and/or modified manually
filter_and_sort_file $actual_file
diff $expected_file $actual_file && echo "[PASS] Analysis output file (output/output.yaml) content is matches to expected output.yaml file." || (echo "[FAIL] Different analysis output, expected output.yaml doesn't match to output/output.yaml."; exit 1)


# Check dependencies
if [ -f dependencies.yaml ]; then
    expected_file=dependencies.yaml
    actual_file=output/dependencies.yaml
    sed 's/^[ \t-]*//' $actual_file | sort -s > $actual_file
    diff $expected_file $actual_file && echo "[PASS] Dependencies (output/dependencies.yaml) content is matches to expected dependencies.yaml file." || (echo "[FAIL] Different dependencies output, expected dependencies.yaml doesn't match to output/dependencies.yaml."; exit 2)
fi

exit 0
