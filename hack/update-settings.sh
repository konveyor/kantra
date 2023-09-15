#!/bin/bash

# Set IMAGE and NAME environment variables, and run the script in root dir:
# $ IMAGE=quay.io/konveyor/kantra NAME=kantra ./hack/update-settings.sh

image="${IMAGE:-quay.io/konveyor/kantra}"
name="${NAME:-kantra}"

sed -i \
    -e "s,\(RootCommandName *string.*default:\"\)[^\"]*\",\1$name\"," \
    -e "s,\(RunnerImage *string.*default:\"\)[^\"]*\",\1$image\"," \
    ./cmd/settings.go
