# Phase 1: Quick Wins (Bug Fixes & Cleanup)

Low-risk fixes that can be done independently.

| # | Fix | File | Risk |
|---|-----|------|------|
| 1 | `log.Fatal` + `os.Exit(1)` redundancy | `cmd/root.go:83-84` | Zero |
| 2 | `CopyFileContents` returns `nil` instead of `err` | `pkg/util/util.go:108` | Low |
| 3 | Typo "depdendecy" | `cmd/analyze.go:468` | Zero |
| 4 | Misleading comment on `WithPortPublish` | `pkg/container/container.go:148` | Zero |
| 5 | Boilerplate copyright placeholder | `cmd/root.go:1-3` | Zero |
| 6 | Remove empty `pkg/provider/internal/local/` | directory | Zero |
| 7 | Remove `import "testing"` from production code | `cmd/root.go:10,40` | Medium |

## Details

### 1. `log.Fatal` + `os.Exit(1)` redundancy

`cmd/root.go:83-84`:
```go
log.Fatal(err, "failed to load global settings")
os.Exit(1)  // unreachable -- log.Fatal already calls os.Exit(1)
```
Remove the `os.Exit(1)`.

### 2. `CopyFileContents` swallows errors

`pkg/util/util.go:105-108`:
```go
source, err := os.Open(src)
if err != nil {
    return nil  // BUG: should be return err
}
```

### 3. Typo "depdendecy"

`cmd/analyze.go:468` -- Change `"depdendecy folder"` to `"dependency folder"`.

### 4. Misleading comment

`pkg/container/container.go:148` -- Comment says `// WithProxy adds proxy environment variables` but the function is `WithPortPublish`. Fix the comment.

### 5. Boilerplate copyright

`cmd/root.go:1-3` -- `Copyright (C) 2023 NAME HERE <EMAIL ADDRESS>` is placeholder text. Replace or remove.

### 6. Empty directory

`pkg/provider/internal/local/` is completely empty. Remove it and its parent `internal/` (which has no other children).

### 7. Remove `import "testing"` from production code

`cmd/root.go:10,40` -- The `testing.Testing()` call exists to handle Go injecting test-specific flags (`--test.v`) that Cobra doesn't recognize.

**Preferred fix:** Use `cobra.Command.FParseErrWhitelist` to tell Cobra to ignore unknown flags. This is the idiomatic Cobra solution for this exact problem.
