# Phase 2: Eliminate Mutable Global State

## Problem A: `util.SourceMountPath` (HIGH PRIORITY)

### What it is

`util.SourceMountPath` is a package-level `var` in `pkg/util/util.go:32`, initialized to `/opt/input/source`. It represents the path where application source code is mounted inside a container.

```go
// pkg/util/util.go:28-32
var (
    M2Dir           = path.Join("/", "root", ".m2")
    SourceMountPath = path.Join(InputPath, "source")  // /opt/input/source
    // ...
)
```

### How it gets mutated

**Mutation site 1 -- `cmd/analyze.go:439`**

During `Validate()`, when input is a file (e.g. a `.war` or `.ear`), the global is permanently modified to include the filename:

```go
// For file inputs, append the filename to the mount path
util.SourceMountPath = path.Join(util.SourceMountPath, filepath.Base(a.input))
// e.g., /opt/input/source --> /opt/input/source/app.war
```

This mutation happens inside validation and has a permanent side effect on all subsequent code.

**Mutation site 2 -- `cmd/analyze-hybrid.go:548-565`**

The hybrid execution path does a fragile save/mutate/restore dance:

```go
originalMountPath := util.SourceMountPath       // save
if a.isFileInput {
    util.SourceMountPath = path.Dir(...)         // mutate for volume mounting
}
err = a.RunProvidersHostNetwork(...)              // use the mutated value
if a.isFileInput {
    util.SourceMountPath = originalMountPath      // restore
}
```

If `RunProvidersHostNetwork` panics, the restore never happens.

### Who reads it (23 call sites)

| File | Usage |
|------|-------|
| `pkg/provider/java.go:21,26` | Reads `SourceMountPath` to set provider `Location` and `workspaceFolders` |
| `pkg/provider/go.go:17` | Reads for `workspaceFolders` config |
| `pkg/provider/python.go:17` | Reads for `workspaceFolders` config |
| `pkg/provider/nodejs.go:17` | Reads for `workspaceFolders` config |
| `pkg/provider/csharp.go:20` | Reads for provider `Location` |
| `pkg/provider/builtin.go:27` | Reads for provider `Location` |
| `cmd/analyze.go:1022` | Volume mount path |
| `cmd/analyze-hybrid.go` | 8 references for provider init, volume mounts, path adjustments |
| `pkg/util/util.go:210` | `GetProfilesExcludedDir()` builds excluded path from it |

### Why this is dangerous

1. **Race conditions** -- If any goroutine or concurrent test accesses `util.SourceMountPath`, you get a data race. There is no synchronization.
2. **Fragile save/restore** -- The pattern in `analyze-hybrid.go` breaks if `RunProvidersHostNetwork` panics or if someone adds an early return between the save and restore.
3. **Non-idempotent validation** -- `Validate()` cannot be called twice safely because the first call permanently mutates the global.
4. **Implicit temporal coupling** -- Providers must be configured *after* `Validate()` runs, but nothing enforces this ordering. The dependency is invisible.
5. **Testability** -- Tests that exercise file-input paths leave the global in a dirty state, potentially affecting subsequent tests.

### Root cause

`SourceMountPath` conflates two different concepts:

1. **Container mount directory** -- Where the source *directory* is mounted in the container. Always `/opt/input/source`.
2. **Source location path** -- Where the actual source *file or directory* lives inside the container. Could be `/opt/input/source` (for directories) or `/opt/input/source/app.war` (for files).

These should be two separate values.

### Proposed fix

#### Step 1: Add `ContainerSourcePath` to the provider interface

In `pkg/provider/provider.go`, add a new field to `ConfigInput`:

```go
type ConfigInput struct {
    // ... existing fields ...
    // ContainerSourcePath is the path to the source inside the container.
    // For directory inputs: /opt/input/source
    // For file inputs:      /opt/input/source/app.war
    ContainerSourcePath string
}
```

#### Step 2: Update all provider implementations

Each provider currently reads `util.SourceMountPath` directly. Change them to use `c.ContainerSourcePath`:

```go
// Before (pkg/provider/go.go):
"workspaceFolders": []interface{}{fmt.Sprintf("file://%s", util.SourceMountPath)},

// After:
"workspaceFolders": []interface{}{fmt.Sprintf("file://%s", c.ContainerSourcePath)},
```

Apply the same change to `java.go`, `python.go`, `nodejs.go`, `csharp.go`, and `builtin.go`.

#### Step 3: Update `GetProfilesExcludedDir`

`pkg/util/util.go:206-215` -- Change the function signature to accept the source path explicitly:

```go
// Before:
func GetProfilesExcludedDir(inputPath string, useContainerPath bool) string {
    if useContainerPath {
        return path.Join(SourceMountPath, ProfilesPath)  // reads global
    }
}

// After:
func GetProfilesExcludedDir(inputPath string, containerSourcePath string, useContainerPath bool) string {
    if useContainerPath {
        return path.Join(containerSourcePath, ProfilesPath)  // uses parameter
    }
}
```

#### Step 4: Compute `sourceLocationPath` in `analyzeCommand`

Add a field to `analyzeCommand` and set it in `Validate()`:

```go
type analyzeCommand struct {
    // ... existing fields ...
    sourceLocationPath string  // computed path inside container
}

// In Validate():
if a.isFileInput {
    a.sourceLocationPath = path.Join(util.SourceMountPath, filepath.Base(a.input))
} else {
    a.sourceLocationPath = util.SourceMountPath
}
// NO mutation of util.SourceMountPath
```

#### Step 5: Pass the path through to providers and hybrid mode

Update `getConfigVolumes()` in `analyze.go` and provider init in `analyze-hybrid.go` to pass `a.sourceLocationPath` via `ConfigInput.ContainerSourcePath`.

Remove the save/restore hack in `analyze-hybrid.go:548-565`. Replace with:

```go
// Volume mount always uses the directory path
mountDir := util.SourceMountPath  // constant: /opt/input/source
// Provider config uses the full path (may include filename for file inputs)
// Already set via a.sourceLocationPath -- no global mutation needed
```

#### Step 6: Make `SourceMountPath` effectively constant

Once no code mutates it, consider making it a `const` or at minimum adding a comment that it must not be mutated. It can't be a true `const` because it's computed via `path.Join`, but the `var` declaration can be documented:

```go
// SourceMountPath is the directory where source code is mounted in the container.
// This value must not be modified at runtime. Use analyzeCommand.sourceLocationPath
// for the resolved source location (which may include a filename for file inputs).
var SourceMountPath = path.Join(InputPath, "source")
```

### Files to modify

| File | Change |
|------|--------|
| `pkg/provider/provider.go` | Add `ContainerSourcePath` field to `ConfigInput` |
| `pkg/provider/java.go` | Use `c.ContainerSourcePath` instead of `util.SourceMountPath` |
| `pkg/provider/go.go` | Use `c.ContainerSourcePath` instead of `util.SourceMountPath` |
| `pkg/provider/python.go` | Use `c.ContainerSourcePath` instead of `util.SourceMountPath` |
| `pkg/provider/nodejs.go` | Use `c.ContainerSourcePath` instead of `util.SourceMountPath` |
| `pkg/provider/csharp.go` | Use `c.ContainerSourcePath` instead of `util.SourceMountPath` |
| `pkg/provider/builtin.go` | Use `c.ContainerSourcePath` instead of `util.SourceMountPath` |
| `pkg/util/util.go` | Add `containerSourcePath` param to `GetProfilesExcludedDir`, add doc comment to `SourceMountPath` |
| `cmd/analyze.go` | Add `sourceLocationPath` field, compute in `Validate()`, remove mutation at line 439, pass to providers |
| `cmd/analyze-hybrid.go` | Use `a.sourceLocationPath` explicitly, remove save/restore hack at lines 548-565 |

### Testing strategy

1. Run `go build ./...` to verify compilation
2. Run `go test ./...` to verify existing tests pass
3. Verify that tests covering file-input paths still work (the `isFileInput` code paths)
4. If integration tests exist for binary analysis (`.war`/`.ear` files), run those

---

## Problem B: Global Variables in `root.go` (LOWER PRIORITY)

### What they are

```go
// cmd/root.go:25-27
var logLevel uint32
var logrusLog *logrus.Logger
var noCleanup bool
```

### Risk assessment

These are **less dangerous** than `SourceMountPath` because:
- They're set once during CLI flag parsing and never mutated during command execution
- They're the standard Cobra pattern for persistent flags

However they still cause friction:
- Tests have to manually reset them (`root_test.go` has 10+ lines of `noCleanup = false`, `logLevel = 4`)
- The `config` package has its own `logLevel *uint32` fields that shadow the global, creating confusion

### Proposed fix

Access flag values via `cmd.Flags().GetUint32("log-level")` inside each command's `RunE`, rather than relying on package-level variables. This can be done incrementally.

This is lower priority and can be deferred to Phase 3 when the God Object is decomposed, since the new sub-package structure will naturally require passing these values explicitly.
