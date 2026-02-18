# Testing Kantra

This guide provides quick local testing commands for developing and validating kantra functionality.

## Prerequisites

```bash
# Build the kantra binary
go build -o kantra .

# Ensure Podman or Docker is running (for hybrid mode)
podman --version  # or docker --version
```

### Environment Variables

Kantra behavior can be customized via environment variables (see `cmd/settings.go`):

**Container Runtime**:
- `CONTAINER_TOOL` — Path to container binary (default: `/usr/bin/podman`, auto-detects podman or docker if not set)
- `PODMAN_BIN` — Legacy alias for `CONTAINER_TOOL` (deprecated, use `CONTAINER_TOOL` instead)

**Container Images**:
- `RUNNER_IMG` — Kantra runner image (default: `quay.io/konveyor/kantra:<version>`)
- `JAVA_PROVIDER_IMG` — Java provider image (default: `quay.io/konveyor/java-external-provider:<version>`)
- `GENERIC_PROVIDER_IMG` — Generic provider image for Go, Python, Node.js (default: `quay.io/konveyor/generic-external-provider:<version>`)
- `CSHARP_PROVIDER_IMG` — C# provider image (default: `quay.io/konveyor/c-sharp-provider:<version>`)

**Runtime Configuration**:
- `RUN_LOCAL` — Set to `true` to force containerless mode (equivalent to `--run-local` flag)
- `JVM_MAX_MEM` — JVM maximum memory for Java analysis (e.g., `4g`, `8g`)
- `CMD_NAME` — Override root command name (default: `kantra`)

## Analysis Commands

### Containerless Mode (Default)

**Basic analysis with Java test data**:
```bash
./kantra analyze \
  --input pkg/testing/examples/ruleset/test-data/java \
  --output ./output-containerless \
  --target cloud-readiness \
  --overwrite
```

**Analysis with custom rules**:
```bash
./kantra analyze \
  --input pkg/testing/examples/ruleset/test-data/java \
  --output ./output-custom-rules \
  --rules pkg/testing/examples/ruleset/local-storage.yml \
  --overwrite
```

### Hybrid Mode (Providers in Containers)

**Basic hybrid analysis**:
```bash
./kantra analyze \
  --input pkg/testing/examples/ruleset/test-data/java \
  --output ./output-hybrid \
  --target cloud-readiness \
  --run-local=false \
  --overwrite
```

**Hybrid with Docker instead of Podman**:
```bash
export CONTAINER_TOOL=/usr/local/bin/docker
./kantra analyze \
  --input pkg/testing/examples/ruleset/test-data/java \
  --output ./output-docker \
  --run-local=false \
  --overwrite
```


## Transform Commands

### OpenRewrite

**List available recipes**:
```bash
./kantra transform openrewrite --list-targets
```

**Run OpenRewrite transformation**:
```bash
./kantra transform openrewrite \
  --input pkg/testing/examples/ruleset/test-data/java \
  --target <recipe-name>
```


## Config Commands

### Login to Hub

**Login with insecure TLS**:
```bash
./kantra config login --insecure https://hub.example.com
```

### Sync Profiles

**Sync from a repository**:
```bash
./kantra config sync --url https://github.com/org/repo
```

**Sync to a specific directory**:
```bash
./kantra config sync \
  --url https://github.com/org/repo \
  --application-path ./my-app
```

**Sync from Hub without auth**:
```bash
./kantra config sync \
  --host https://hub.example.com \
  --url https://github.com/org/repo
```

### List Profiles

**List profiles in specific directory**:
```bash
./kantra config list --profile-dir ./my-app
```

## Test Commands (YAML Rules)

**Test a single rule file**:
```bash
./kantra test pkg/testing/examples/ruleset/local-storage.test.yml
```

**Test all rules in a directory**:
```bash
./kantra test pkg/testing/examples/ruleset/
```

**Test with specific labels**:
```bash
./kantra test \
  --rules pkg/testing/examples/ \
  --labels "konveyor.io/target=cloud-readiness"
```

## Debugging Tips

**Enable verbose logging**:
```bash
./kantra analyze --input . --output ./out --log-level 0
```

**Keep containers running after analysis (hybrid mode)**:
```bash
export KANTRA_NO_CLEANUP=1
./kantra analyze --input . --output ./out --run-local=false
```

## Testing Scenarios

### Quick Smoke Test (Containerless)

```bash
# Build and run basic analysis
go build -o kantra .
./kantra analyze \
  --input pkg/testing/examples/ruleset/test-data/java \
  --output ./test-output \
  --target cloud-readiness \
  --overwrite

# Check output exists
ls -la test-output/
cat test-output/output.yaml | head -20
```

### Quick Smoke Test (Hybrid)

```bash
# Ensure Podman is running
podman ps

# Run hybrid analysis
./kantra analyze \
  --input pkg/testing/examples/ruleset/test-data/java \
  --output ./test-hybrid \
  --run-local=false \
  --overwrite

# Verify output
ls -la test-hybrid/
```

## Clean Up

```bash
# Remove test outputs
rm -rf ./output-* ./test-* ./rule-test ./traced

# Clean up containers (if needed)
podman ps -a | grep konveyor | awk '{print $1}' | xargs podman rm -f

# Clean up images (if needed)
podman images | grep konveyor | awk '{print $3}' | xargs podman rmi -f
```