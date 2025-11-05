# Hybrid Containerized Architecture Implementation Plan

## Problem Statement

Current containerized mode (`--run-local=false`) on macOS exhibits **3.7x performance degradation** compared to containerless mode:
- **Containerless**: 34.6s (465% CPU utilization)
- **Containerized**: 127.9s (0% CPU - I/O bound)

Root cause: Docker/Podman volume mounts on macOS traverse a VM boundary, creating severe I/O latency for file operations.

## Current Architecture

### Containerless Mode (`--run-local=true`)
```
┌─────────────────────────────────────────┐
│  Host (kantra CLI)                      │
│    ├─> analyzer-lsp (Go library)        │
│    │   - In-process execution           │
│    │   - Direct file I/O                │
│    │                                     │
│    └─> Java provider (in-process)       │
│        - Native execution                │
│        - Direct memory access            │
└─────────────────────────────────────────┘
```

**Performance**: ✅ Fast (34.6s)
**Isolation**: ❌ None - all processes on host
**Portability**: ❌ Requires Java/dependencies on host

### Current Containerized Mode (`--run-local=false`)
```
┌─────────────────────────────────────────┐
│  Host (kantra CLI)                      │
│    └─> Orchestrates containers          │
└─────────────────────────────────────────┘
           │
           ├─> Container: konveyor-analyzer
           │   - Volume: input/ (SLOW)
           │   - Volume: output/ (SLOW)
           │   - Reads source via VM boundary
           │   - Writes results via VM boundary
           │   │
           │   └─> Network: localhost:9001
           │
           └─> Container: java-provider
               - Volume: input/ (SLOW)
               - Listens on port 9001
```

**Performance**: ❌ Slow (127.9s) - volume I/O bottleneck
**Isolation**: ✅ Full container isolation
**Portability**: ✅ Works anywhere with container runtime

## Proposed Hybrid Architecture

### Design Overview
Run analyzer-lsp on the **host** (like containerless), but keep providers **containerized** with network communication:

```
┌─────────────────────────────────────────┐
│  Host (kantra CLI)                      │
│    ├─> analyzer-lsp (Go library)        │
│    │   - In-process execution           │
│    │   - Direct file I/O ⚡ FAST        │
│    │   - Connects to localhost:9001     │
│    │                                     │
│    └─> Start provider containers        │
└─────────────────────────────────────────┘
           │
           └─> Container: java-provider
               - Volume: input/ (for provider only)
               - Listens on localhost:9001
               - Network: host mode
```

**Performance**: ✅ Fast (~40s expected) - direct file I/O
**Isolation**: ⚡ Partial - providers isolated, analyzer on host
**Portability**: ⚡ Hybrid - needs Go binary, containerizes providers

### Key Benefits

1. **Eliminates volume mount overhead** for analyzer file I/O
2. **Reuses existing provider infrastructure** (already network-capable)
3. **Minimal code changes** - combines existing containerless + container logic
4. **Backward compatible** - can coexist with current modes
5. **Performance parity with containerless** on macOS

## Implementation Plan

### Phase 1: Code Analysis & Preparation

**Files to Review:**
- `cmd/analyze.go` - Current containerized mode logic
- `cmd/analyze-bin.go` - Current containerless mode logic (reference implementation)
- `pkg/provider/*.go` - Provider initialization

**Key Questions to Answer:**
1. How do providers differentiate between in-process vs. network connections?
2. What network settings are needed for localhost provider communication?
3. Do providers need volume mounts if analyzer runs on host?

### Phase 2: Core Implementation

#### 2.1 Modify Provider Startup for Network Mode

**File**: `cmd/analyze.go` - `RunProviders()` function

**Current behavior**: Starts providers in containers with `--port=9001`

**Changes needed**:
```go
// Add network mode to connect via localhost
container.WithNetwork("host")  // Instead of container network
// Providers will be accessible at localhost:9001, localhost:9002, etc.
```

#### 2.2 Create Network-Based Provider Clients

**File**: New file `cmd/provider-clients.go` or modify existing provider setup

**Current behavior**: Containerless uses in-process provider clients

**Changes needed**:
```go
// Instead of java.NewJavaProvider() for in-process
// Create gRPC client that connects to localhost:9001

func (a *analyzeCommand) setupNetworkJavaProvider(ctx context.Context, port int) (provider.InternalProviderClient, error) {
    // Connect to localhost:port via gRPC
    // Return provider client that communicates over network
}
```

**Research needed**: Check if analyzer-lsp already has network provider client implementation

#### 2.3 Modify RunAnalysis to Use Host Execution

**File**: `cmd/analyze.go` - `RunAnalysis()` function

**Current behavior** (~line 1207-1330):
```go
func (a *analyzeCommand) RunAnalysis(ctx context.Context, volName string) error {
    // Sets up volumes
    // Creates analyzer container
    // Runs konveyor-analyzer in container
}
```

**Changes needed**:
```go
func (a *analyzeCommand) RunAnalysisHybrid(ctx context.Context) error {
    // 1. Start provider containers (keep existing RunProviders logic)
    err := a.RunProviders(ctx, networkName, volName, 5)

    // 2. Setup network provider clients (NEW)
    providers := a.setupNetworkProviders(ctx)

    // 3. Run analyzer on host (borrow from analyze-bin.go)
    eng := engine.CreateRuleEngine(ctx, ...)

    // 4. Execute analysis with direct file I/O
    // (No container, no volume mounts for analyzer)
}
```

#### 2.4 File I/O Changes

**Volume mounts needed**:
- ❌ **Remove**: Analyzer container volumes (input, output)
- ✅ **Keep**: Provider container volumes (they still need source access)

**File access**:
- Analyzer: Direct host filesystem access
- Providers: Via mounted volumes (only they need containers)

### Phase 3: Integration Points

#### 3.1 Provider Settings

**File**: `cmd/analyze.go` - Provider configuration

**Current**: Provider settings passed via mounted JSON file
**Hybrid**: Pass settings via:
- Option A: Network (gRPC metadata)
- Option B: Still use temp file on host, provider reads from volume
- **Recommendation**: Option B (simpler, less invasive)

#### 3.2 Progress Reporting

**File**: `cmd/analyze.go` - Progress tracking

**Current containerized**: Streams from container stderr
**Hybrid**: Use in-process progress like containerless mode

Reuse existing progress bar implementation from `analyze-bin.go`:
```go
// Lines 64-81: renderProgressBar()
// Lines 267-286: Progress event handling
```

#### 3.3 Cleanup & Resource Management

**Containers to cleanup**:
- Provider containers (keep existing cleanup logic)
- ❌ Analyzer container (no longer created)

**Volumes to cleanup**:
- Provider volumes (keep)
- ❌ Analyzer volumes (no longer created)

### Phase 4: Testing Strategy

#### 4.1 Functional Testing

**Test matrix**:
```
| Test Case                    | Expected Result                |
|------------------------------|--------------------------------|
| Basic analysis               | Same output as containerless   |
| Multiple providers           | All providers accessible       |
| Large codebase              | Direct I/O works correctly     |
| Error handling              | Graceful provider failures     |
| Progress reporting          | Clean progress bar display     |
```

#### 4.2 Performance Benchmarking

**Baseline comparisons**:
```bash
# Containerless (baseline)
time ./bin/kantra analyze --run-local=true ...
# Expected: ~34s

# Current containerized (slow)
time ./bin/kantra analyze --run-local=false ...
# Current: ~127s

# Hybrid (target)
time ./bin/kantra analyze --run-local=false ...
# Target: ~40s (allows for some provider overhead)
```

**Success criteria**: Hybrid mode should be within 20% of containerless performance

#### 4.3 Compatibility Testing

**Platforms to test**:
- ✅ macOS (primary target - has VM overhead)
- ✅ Linux (should work, less benefit)
- ⚠️  Windows (may need special handling)

**Container runtimes**:
- Docker
- Podman

### Phase 5: Migration Path

#### 5.1 Flag Strategy

**Option A**: New flag (recommended for safety)
```bash
--run-hybrid=true  # New hybrid mode
--run-local=true   # Existing containerless
--run-local=false  # Existing containerized
```

**Option B**: Auto-detect and use hybrid by default
```bash
--run-local=false  # Automatically uses hybrid mode
--run-local=false --no-hybrid  # Fallback to old containerized
```

**Recommendation**: Start with Option A for testing, migrate to Option B after validation

#### 5.2 Backward Compatibility

- Keep existing containerized mode as fallback
- Add deprecation notice for old containerized mode
- Provide migration guide in documentation

## Implementation Checklist

### Research Phase
- [ ] Investigate analyzer-lsp provider network client support
- [ ] Check if providers work with `--network=host` mode
- [ ] Verify gRPC endpoint configuration
- [ ] Test provider volume mount requirements

### Development Phase
- [ ] Create `setupNetworkProviders()` function
- [ ] Modify `RunProviders()` to use host network
- [ ] Create `RunAnalysisHybrid()` function
- [ ] Borrow engine initialization from `analyze-bin.go`
- [ ] Implement progress bar for hybrid mode
- [ ] Update provider configuration passing

### Testing Phase
- [ ] Unit tests for network provider clients
- [ ] Integration test: basic analysis
- [ ] Integration test: multi-provider setup
- [ ] Performance benchmark: macOS
- [ ] Performance benchmark: Linux
- [ ] Edge case: provider startup failures
- [ ] Edge case: network connectivity issues

### Documentation Phase
- [ ] Update README with hybrid mode usage
- [ ] Document performance characteristics
- [ ] Add troubleshooting guide
- [ ] Update architecture diagrams

## Risk Assessment

### High Risk
- **Provider network compatibility**: Providers may not support host network mode
  - *Mitigation*: Test provider connectivity early

- **File path translation**: Providers in containers may have different paths
  - *Mitigation*: Ensure volume mounts preserve paths

### Medium Risk
- **Platform differences**: Behavior may vary on Linux vs macOS vs Windows
  - *Mitigation*: Test on multiple platforms

- **Breaking changes**: May affect downstream tools (kai, etc.)
  - *Mitigation*: Coordinate with maintainers

### Low Risk
- **Performance regression**: Hybrid mode could be slower than expected
  - *Mitigation*: Benchmark early, optimize if needed

## Success Metrics

1. **Performance**: Hybrid mode within 20% of containerless speed
2. **Compatibility**: Works on macOS, Linux with Docker/Podman
3. **Reliability**: Passes all existing integration tests
4. **Usability**: Clear documentation and migration path

## Timeline Estimate

- **Research & Design**: 1 day
- **Core Implementation**: 2-3 days
- **Testing & Debugging**: 2 days
- **Documentation**: 1 day

**Total**: ~1 week for complete implementation

## Open Questions

1. Do analyzer-lsp providers already support network mode, or do we need to implement it?
2. Should we make hybrid mode the default for `--run-local=false`?
3. How should we handle platforms where volume performance is acceptable?
4. Should we deprecate old containerized mode entirely?

## References

- Containerless implementation: `cmd/analyze-bin.go`
- Container orchestration: `cmd/analyze.go`
- Provider interfaces: `pkg/provider/*.go`
- analyzer-lsp library: `github.com/konveyor/analyzer-lsp`
