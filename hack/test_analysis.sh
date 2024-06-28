#!/bin/sh
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
function filter_and_sort() {
  yq e 'del(.[].skipped) | del(.[].unmatched)' $1 \
    | yq e '.[]?.violations |= (. | to_entries | sort_by(.key) | from_entries)' \
    | yq e '.[]?.violations[]?.incidents |= sort_by(.uri)' \
    | yq e '.[] | (.tags // []) |= sort'
}
filter_and_sort $expected_file > $expected_file
filter_and_sort $actual_file > $actual_file
diff $expected_file $actual_file


# Check dependencies
if [ -f dependencies.yaml ]; then
    expected_file=dependencies.yaml
    actual_file=output/dependencies.yaml
    sed 's/^[ \t-]*//' $expected_file | sort -s > /tmp/expected_file
    sed 's/^[ \t-]*//' $actual_file | sort -s > /tmp/actual_file
    diff /tmp/expected_file /tmp/actual_file || diff $expected_file $actual_file
fi

exit 0
