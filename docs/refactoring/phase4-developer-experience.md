# Phase 4: Developer Experience (Makefile + Test Organization)

## Prerequisites

- None -- this phase is largely independent of Phases 1-3
- Phase 3b (if completed) reduces the scope of test file splitting since tests move during the `cmd/analyze/` migration

---

## Part A: Add a Makefile

### Current state

There is no Makefile. The build process is scattered across:

- **CI (`testing.yaml:58`)** -- `go build -o kantra main.go` with no ldflags (missing version/commit info)
- **Dockerfile (`Dockerfile:38-54`)** -- `go build` with 8 `-X` ldflags repeated 3 times for linux/darwin/windows:
  ```
  -X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$VERSION'
  -X 'github.com/konveyor-ecosystem/kantra/cmd.RunnerImage=$IMAGE'
  -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$BUILD_COMMIT'
  -X 'github.com/konveyor-ecosystem/kantra/cmd.JavaBundlesLocation=$JAVA_BUNDLE'
  -X 'github.com/konveyor-ecosystem/kantra/cmd.JavaProviderImage=$JAVA_PROVIDER_IMG'
  -X 'github.com/konveyor-ecosystem/kantra/cmd.DotnetProviderImage=$DOTNET_PROVIDER_IMG'
  -X 'github.com/konveyor-ecosystem/kantra/cmd.GenericProviderImage=$GENERIC_PROVIDER_IMG'
  -X 'github.com/konveyor-ecosystem/kantra/cmd.RootCommandName=$NAME'
  ```
- **Tests** -- `go test ./...` runs everything (no unit/integration separation)
- **Lint** -- No `.golangci.yml` or lint configuration exists

### Proposed Makefile targets

| Target | Command | Notes |
|--------|---------|-------|
| `make build` | `go build` with proper ldflags | Mirrors Dockerfile ldflags; injects `Version`, `BuildCommit`, `RunnerImage` |
| `make test` | `go test ./...` (unit only, after build tags are added) | Fast feedback loop for development |
| `make test-integration` | `go test -tags=integration ./...` | Requires container runtime (podman/docker) |
| `make test-all` | Unit tests + integration tests + Ginkgo | Full CI equivalent |
| `make lint` | `golangci-lint run` | Requires adding `.golangci.yml` |
| `make clean` | Remove build artifacts | |
| `make docker-build` | `podman build -t localhost/kantra:latest .` | Local container image build |

### Ldflags consolidation

The ldflags string is currently copy-pasted 3 times in the Dockerfile (lines 38, 44, 50 -- one per GOOS). The Makefile would define them once as a variable:

```makefile
VERSION     ?= latest
BUILD_COMMIT = $(shell git rev-parse --short HEAD)
IMAGE       ?= quay.io/konveyor/kantra
NAME        ?= kantra

LDFLAGS = -X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$(VERSION)' \
          -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$(BUILD_COMMIT)' \
          -X 'github.com/konveyor-ecosystem/kantra/cmd.RunnerImage=$(IMAGE)' \
          -X 'github.com/konveyor-ecosystem/kantra/cmd.RootCommandName=$(NAME)'

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o kantra main.go
```

The Dockerfile can then be simplified to use the Makefile or at least reference the same variable pattern, eliminating the 3x repetition.

### Implementation

1. Create `Makefile` in the repo root
2. Optionally create `.golangci.yml` with a reasonable default config
3. Update CI `testing.yaml` to use `make test` / `make test-integration` instead of raw `go test` commands
4. Update `CONTRIBUTING.md` or `README.md` to document the new targets

---

## Part B: Test Organization

### Current problems

1. **No separation between unit and integration tests** -- Zero `//go:build` tags across 33 test files. Integration tests use runtime `t.Skip()` guards based on container runtime availability. CI labels the step "Run unit tests" but runs everything including container-dependent tests.

2. **Three test frameworks with no standard** -- stdlib `testing` (25 files), testify (2 files), Ginkgo/Gomega (8 files in `asset_generation/` only). The testify usage in `cmd/analyze-bin_test.go` and `pkg/provider/java_test.go` is inconsistent with the rest of the codebase.

3. **Oversized test files** -- `cmd/analyze_test.go` (2,120 lines, 20 test funcs), `cmd/config/config_test.go` (1,941 lines, 26 test funcs).

4. **No shared test utilities** -- Helpers like `getContainerBinary()`, `volumeExists()`, `generateMockJWT()` are inlined in individual test files and only usable within `package cmd`.

5. **No `testing.Short()` support** -- There is no lightweight "fast test" mode for rapid development iteration.

6. **CI mislabeling** -- `testing.yaml:68` says "Run unit tests" but runs `go test ./...` which includes integration tests.

### Fix 1: Add `//go:build integration` tags

Add build tags to integration test files so `go test ./...` skips them by default:

**Files to tag:**
- `cmd/maven_cache_integration_test.go` (386 lines, 6 test funcs)
- `cmd/hybrid_integration_test.go` (308 lines, 6 test funcs)

**Tests to extract from regular test files into new `*_integration_test.go` files:**
- Integration-like tests in `cmd/analyze_test.go` that check for container runtime with `t.Skip()`
- Integration-like tests in `cmd/analyze-bin_test.go` that check for container runtime with `t.Skip()`

After this, `go test ./...` runs only true unit tests. `go test -tags=integration ./...` runs everything.

### Fix 2: Fix CI workflow labels

Split the single "Run unit tests" step in `testing.yaml:68-71` into two:

```yaml
- name: Run unit tests
  run: make test

- name: Run integration tests
  run: make test-integration
```

This makes CI failures easier to triage -- you immediately know whether a unit or integration test failed.

### Fix 3: Create `internal/testutil/` package

Extract duplicated test helpers into a shared package usable across both `cmd/` and `pkg/`:

**From `cmd/maven_cache_integration_test.go`:**
- `getContainerBinary() string` -- finds podman or docker binary
- `isContainerRuntimeAvailable() bool` -- checks if runtime is functional
- `volumeExists(volumeName string) bool` -- checks if a container volume exists
- `cleanupMavenCacheVolume(t *testing.T)` -- removes test volumes

**From `cmd/config/config_test.go`:**
- `generateMockJWT(expirationTime) string` -- creates mock JWT tokens for auth tests
- `setupTempAuth(t, auth)` -- creates temp HOME with auth data

Currently these are only usable within `package cmd` because they're defined in `_test.go` files. A shared `internal/testutil/` package makes them available across package boundaries.

### Fix 4: Split oversized test files

**Note:** If Phase 3b has already been completed, test files were split during the `cmd/analyze/` migration and this step is already done.

If Phase 3b has not happened yet, these are the targets:

**`cmd/analyze_test.go` (2,120 lines, 20 test funcs)** -- split by concern:
- `analyze_validation_test.go` -- `Test_analyzeCommand_Validate*`, `Test_analyzeCommand_validateRulesPath`
- `analyze_profile_test.go` -- `Test_analyzeCommand_ValidateAndLoadProfile`
- `analyze_labels_test.go` -- `Test_analyzeCommand_getLabelSelectorArgs`
- `analyze_rules_test.go` -- `Test_analyzeCommand_needDefaultRules`
- `analyze_maven_test.go` -- `Test_analyzeCommand_RunAnalysis_MavenSearch`
- `analyze_output_test.go` -- `Test_analyzeCommand_RunAnalysis_InputValidation`

**`cmd/config/config_test.go` (1,941 lines, 26 test funcs)** -- split by concern:
- `config_loading_test.go` -- config file loading and parsing
- `config_hub_test.go` -- hub client HTTP interactions
- `config_auth_test.go` -- JWT auth and token management
- `config_profile_test.go` -- profile downloading and syncing

### Fix 5: Standardize assertion approach

Two files use testify (`cmd/analyze-bin_test.go`, `pkg/provider/java_test.go`) while 25 files use raw `if/t.Errorf`. Pick one approach:

**Option A (recommended): Standardize on testify** -- More concise assertions, better error messages. Migrate the 25 files using raw assertions to `assert`/`require`. This is a mechanical change.

**Option B: Remove testify** -- Convert the 2 testify files to raw assertions. Less dependency, but more verbose test code.

The Ginkgo/Gomega usage in `cmd/asset_generation/` is cleanly isolated with its own suite bootstraps and can remain as-is.

### Fix 6: Add `testing.Short()` support

Add `if testing.Short() { t.Skip("skipping slow test") }` to tests that:
- Run full analysis pipelines
- Build or interact with containers
- Download external dependencies
- Take more than a few seconds

This enables `go test -short ./...` for rapid iteration during development without waiting for slow tests.

---

## Implementation order

1. **Create `Makefile`** -- immediate value, no dependencies
2. **Add `//go:build integration` tags** -- quick, low-risk
3. **Fix CI workflow** -- depends on Makefile existing
4. **Create `internal/testutil/`** -- extract shared helpers
5. **Add `testing.Short()` guards** -- mechanical
6. **Split oversized test files** -- skip if Phase 3b handles this
7. **Standardize assertions** -- lowest priority, mechanical

Each item can be committed independently.

---

## Dependencies

```
Phase 4 (this phase) is independent of Phases 1-3

However:
  - Phase 3b (cmd/analyze/ sub-package) → makes Fix 4 unnecessary (tests split during migration)
  - Makefile creation → CI workflow update depends on it
  - Build tags → Makefile test targets depend on them
```
