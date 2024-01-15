#!/bin/bash

# Set IMAGE and NAME environment variables, and run the script in root dir:
# $ IMAGE=quay.io/konveyor/kantra NAME=kantra ./hack/update-settings.sh

image="${IMAGE:-quay.io/konveyor/kantra}"
name="${NAME:-kantra}"
bundle="${BUNDLE:-/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar}"

sed -i \
    -e "s,\(RootCommandName *string.*default:\"\)[^\"]*\",\1$name\"," \
    -e "s,\(RunnerImage *string.*default:\"\)[^\"]*\",\1$image\"," \
    -e "s,\(JavaBundlesLocation *= *\"\)[^\"]*\",\1$bundle\"," \
    ./cmd/settings.go
