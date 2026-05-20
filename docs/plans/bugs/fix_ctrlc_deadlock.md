# Fix Ctrl-C Deadlock During Analysis (Issue #776)
* https://github.com/konveyor/kantra/issues/776

## Problem

`kantra analyze` hangs when Ctrl-C is pressed at various phases of analysis. Multiple independent bugs prevent the signal context cancellation from propagating to blocking operations.

## Hang Paths Identified

We found and fixed hang paths at three distinct phases:

### Phase A: During provider startup (NewAnalyzer / NewGRPCClient)

**Symptom:** Ctrl-C right after "Started providers" is completely ignored. Process hangs indefinitely.

**Root cause:** `NewGRPCClient` (`provider/grpc/provider.go:172`) creates the provider subprocess and gRPC connection using `context.WithCancel(context.Background())`. This is entirely disconnected from the signal context. Ctrl-C cancels the signal context but `NewGRPCClient` never sees it — the subprocess keeps running and the gRPC connection/reflection checks keep waiting.

**Root cause 2:** `checkServicesRunning` (`provider/grpc/provider.go:266-282`) is an infinite retry loop that polls for gRPC service availability after the provider binary starts. It takes no context, so it cannot be interrupted by Ctrl-C. It also has a broken 30-second timeout: the `time.After(30s)` was created inside the `for` loop (reset every iteration) and placed in a `select` alongside a `default` case (which always wins). The timeout could never fire.

**Fixes:**
- Change 7: Thread signal-aware context through `GetProviderClient` → `NewGRPCClient`. The provider subprocess (`exec.CommandContext`) and gRPC reflection check now derive from the signal context.
- Change 8: Rewrite `checkServicesRunning` to accept `context.Context`, check `ctx.Done()` in the select and during the 3-second retry sleep, and move the 30-second deadline outside the loop so it actually works.

**Status:** Implemented. API changed: `GetProviderClient` and `NewGRPCClient` now accept `context.Context` as first parameter. `checkServicesRunning` now accepts `context.Context` as first parameter.

### Phase B: During provider init/prepare (ProviderStart)

**Symptom:** Ctrl-C during "Preparing provider" phase hangs for up to 8 minutes.

**Root causes:**
1. `abConfigChan` in `ProviderStart()` (`core/analyzer.go:144`) is unbuffered. Init goroutines send on it (line 182), but if the collector goroutine has exited via `ctx.Done()`, the send blocks forever and the waitGroup never drains. Fixed by wrapping the send in a `select` with `providerInitCtx.Done()` and moving `waitGroup.Done()` to the sender side (after the select) so it always runs regardless of cancellation.
2. The init wait select (lines 204-215) has no `ctx.Done()` case — it only checks `<-c` (all providers done) and `time.After(timeout)`. Cancellation is not detected until the 8-minute default timeout expires.
3. `grpcServiceClient.Stop()` (`provider/grpc/service_client.go:206`) uses `context.TODO()` for the gRPC Stop call. During cleanup after cancellation, this can hang indefinitely if the provider is unresponsive.

**Fixes:**
- Change 5a: Wrap `abConfigChan` send in `select` with `providerInitCtx.Done()`, move `waitGroup.Done()` to sender
- Change 5b: Add `a.ctx.Done()` case to the init wait select
- Change 6: Use `context.WithTimeout(context.Background(), 5*time.Second)` in `grpcServiceClient.Stop()`

**Status:** Implemented.

### Phase C: During rule execution (RunRulesScopedWithOptions)

**Symptom:** Ctrl-C during "Processing rules" causes ~10-15 second delay before exit. Previously caused permanent hang.

**Root causes:**
1. The rule dispatch loop (`engine/engine.go:355-364`) sends to `r.ruleProcessing` (buffered at 10) without checking `ctx.Done()`. When workers exit on cancellation, the channel fills and the loop blocks forever.
2. The response channel `ret` (line 289) is unbuffered. The response-handler goroutine exits on `ctx.Done()`. Workers that finish a rule then block forever on `m.returnChan <- response` because nobody is receiving.

**Fixes:**
- Change 1: Buffer response channel to `len(otherRules)`
- Change 2: Wrap dispatch send in `select` with `ctx.Done()` case

**Status:** Implemented. The ~10 second delay after Ctrl-C during rule processing is likely from `anlzr.Stop()` → provider `Stop()` gRPC calls. Change 6 (5-second timeout) bounds this.

### Additional kantra-side fixes

- **Change 3:** After `anlzr.Run()` returns, check `ctx.Err()` and return immediately if cancelled. Prevents dependency resolution, output writing, and static report generation from running after Ctrl-C.
- **Change 4:** `env.Stop()` uses a fresh `context.WithTimeout(context.Background(), 30s)` instead of the already-cancelled signal context. Ensures container stop/remove and volume cleanup actually execute.

**Status:** Implemented.

## All Changes Summary

| # | Repo | File | What | Why |
|---|------|------|------|-----|
| 1 | analyzer-lsp | `engine/engine.go:289` | Buffer response channel `make(chan response, len(otherRules))` | Workers can send responses after handler exits |
| 2 | analyzer-lsp | `engine/engine.go:355-364` | `select` with `ctx.Done()` in dispatch loop | Dispatch loop can exit on cancellation |
| 3 | kantra | `cmd/analyze/run.go` (after `anlzr.Run()`) | Check `ctx.Err()`, return error | Skip post-analysis work after cancel |
| 4 | kantra | `cmd/analyze/run.go:143` | Fresh 30s timeout context for `env.Stop()` | Cleanup works with cancelled signal context |
| 5a | analyzer-lsp | `core/analyzer.go:144,181-185` | Wrap `abConfigChan` send in `select` with `providerInitCtx.Done()`; move `waitGroup.Done()` to sender | Init goroutines don't block on cancelled collector; channel stays unbuffered (more idiomatic Go) |
| 5b | analyzer-lsp | `core/analyzer.go:204-215` | Add `a.ctx.Done()` to init wait select | Cancellation detected immediately, not after 8min timeout |
| 6 | analyzer-lsp | `provider/grpc/service_client.go:206` | 5s timeout on gRPC Stop call | Cleanup doesn't hang on unresponsive provider |
| 7 | analyzer-lsp | `provider/grpc/provider.go:172`, `provider/lib/lib.go`, `core/types.go`, `cmd/dep/main.go` | Thread `context.Context` through `GetProviderClient` → `NewGRPCClient` | Provider startup respects signal cancellation |
| 8 | analyzer-lsp | `provider/grpc/provider.go:266-282` | Rewrite `checkServicesRunning` to accept context, fix broken timeout | Service check loop can be interrupted by cancellation |
| 9 | kantra | `cmd/analyze/analyze.go:126-150` | Dedicated signal channel for interrupt feedback + force-exit on second Ctrl-C | Immediate user feedback + escape hatch for slow cleanup. Uses `signal.Notify` on a separate channel (not `ctx.Done()`) so normal context cancellation on clean exit does not trigger misleading output or leak signal state. Goroutine exits cleanly via `analysisDone` channel on normal shutdown. |

## Context Chain (after fixes)

```
signal.NotifyContext(context.Background(), os.Interrupt)     ← analyze.go:126
  └─ context.WithCancel(ctx)                                  ← run.go:234
      └─ context.WithCancel(opts.ctx)                         ← NewAnalyzer types.go:109
          ├─ GetProviderClient(ctx, ...)                      ← types.go:117 [NEW: Change 7]
          │   └─ NewGRPCClient(ctx, ...)                      ← lib.go:17 [NEW: Change 7]
          │       └─ context.WithCancel(ctx)                   ← provider.go:175 [was: context.Background()]
          │           ├─ exec.CommandContext(ctxCmd, ...)      ← start() - provider subprocess
          │           └─ checkServicesRunning(ctx, ...)        ← provider.go [NEW: Change 8]
          ├─ startable.Start(ctx)                             ← types.go:124
          ├─ ProviderStart()  uses a.ctx                      ← analyzer.go:129
          │   ├─ abConfigChan send: select { case <-providerInitCtx.Done() } [NEW: Change 5a]
          │   └─ select { case <-a.ctx.Done() } [NEW: Change 5b]
          └─ CreateRuleEngine(ctx, ...)                       ← types.go:148
              └─ context.WithCancel(ctx)                       ← engine.go:141
                  ├─ processRuleWorker: select { case <-ctx.Done() }
                  └─ RunRulesScopedWithOptions:
                      ├─ dispatch: select { case <-ctx.Done() } [NEW: Change 2]
                      └─ ret buffered [NEW: Change 1]
```

## Tests Added

### analyzer-lsp: `engine/engine_test.go`

1. **`TestRunRules_CancelDuringDispatch`** — 1 worker, 20 slow rules (exceeds buffer). Cancel after 100ms. Assert RunRules returns within 10s. Validates changes 1+2.
2. **`TestRunRules_CancelDuringExecution`** — 10 workers, 10 slow rules. Cancel after 500ms. Assert returns within 10s. Validates change 1.
3. **`TestEngineStop_AfterCancelledRun`** — Run with cancelled context, then call Stop(). Assert Stop returns within 5s.

### kantra: `cmd/analyze/providers_test.go`

4. **`Test_setupProgressReporter_ShutdownOnCancel`** — Cancel progress context, assert done channel closes within 5s.

## Current Status

All 9 changes are implemented. All tests pass:
- `analyzer-lsp`: `go test ./engine/... ./core/... ./provider/grpc/... -count=1` ✓
- `kantra`: `go test ./cmd/analyze/... -count=1` ✓
- Both repos build ✓

### Manual testing observations

- **Ctrl-C during rule processing:** Now exits with ~10s delay (down from permanent hang). The delay is from workers draining in-flight gRPC Evaluate calls plus up to 5s provider Stop timeout (change 6). Change 9 uses a dedicated signal channel (not `ctx.Done()`) to print "Interrupt received, shutting down..." only on a real SIGINT, and a second Ctrl-C force-exits instantly. On normal exit, the goroutine exits cleanly via an `analysisDone` channel without printing anything.
- **Ctrl-C during early provider startup:** Previously ignored entirely. Changes 7+8 address this. Change 7 threads the signal context into `NewGRPCClient`. Change 8 fixes `checkServicesRunning` — an infinite retry loop that polled for gRPC services with no context and a broken 30s timeout (timer reset every loop iteration, `default` case always won over `time.After`). Now accepts context and checks `ctx.Done()`. Needs manual verification after rebuild.
- **Ctrl-C during provider prepare:** Previously hung for up to 8 minutes. Changes 5a/5b address this — needs manual verification.

## Local Development Setup

```sh
# In kantra repo, point at local analyzer-lsp
go mod edit -replace github.com/konveyor/analyzer-lsp=../analyzer-lsp
go build -o ./kantra .

# Test
go test ./cmd/analyze/... -count=1

# In analyzer-lsp repo
go test ./engine/... ./core/... ./provider/... -count=1

# When done, remove replace
go mod edit -dropreplace github.com/konveyor/analyzer-lsp
```

## Future Hardening (not required for this fix)

- Per-run context propagation into `ruleMessage` so workers use the run context for gRPC Evaluate calls
- `ctx.Done()` checks in `runTaggingRules` and condition evaluation loops
- Progress goroutine timeout guard in `runAnalysis`
