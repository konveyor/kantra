# Research Findings: Hybrid Architecture Network Communication

## Key Discovery: `provider.Config.Address` Field

### Provider Configuration Modes

The analyzer-lsp library supports **two modes** of provider communication via the `provider.Config` struct:

```go
type Config struct {
    Name         string       // Provider name (e.g., "java", "builtin")
    BinaryPath   string       // Path to provider binary (IN-PROCESS mode)
    Address      string       // Network address (NETWORK mode) ⭐
    InitConfig   []InitConfig // Provider initialization config
    // ... other fields
}
```

### Mode 1: In-Process (Containerless)
**Used by**: `--run-local=true` (containerless mode)

```go
javaConfig := provider.Config{
    Name:       "java",
    BinaryPath: "/path/to/jdtls",  // ← Provider runs in-process
    Address:    "",                 // ← Empty = in-process
    InitConfig: [...]
}
```

**How it works**:
- Analyzer imports provider as Go library
- Provider runs in same process
- Direct function calls (no network)
- Fast but requires dependencies on host

### Mode 2: Network/gRPC (Proposed Hybrid)
**Will be used by**: Hybrid containerized mode

```go
javaConfig := provider.Config{
    Name:       "java",
    BinaryPath: "",                    // ← Empty for network mode
    Address:    "localhost:9001",      // ← Provider address ⭐
    InitConfig: [...]
}
```

**How it works**:
- Analyzer creates gRPC client
- Provider runs in container with `--port=9001`
- Communication via localhost network
- Fast file I/O, isolated providers

## Provider Network Support

### Confirmed: Providers Already Support Network Mode

From `go doc github.com/konveyor/analyzer-lsp/provider`:

```go
type ServiceClient interface {
    Evaluate(ctx context.Context, cap string, conditionInfo []byte) (ProviderEvaluateResponse, error)
    GetDependencies(ctx context.Context) (map[uri.URI][]*Dep, error)
    GetDependenciesDAG(ctx context.Context) (map[uri.URI][]DepDAGItem, error)
    NotifyFileChanges(ctx context.Context, changes ...FileChange) error
    Stop()
}
```

### Provider Client Hierarchy

```
InternalProviderClient (used in containerless)
├─ InternalInit
└─ Client
   ├─ BaseClient
   └─ ServiceClient ← Core interface for provider operations
```

### How Analyzer Creates Clients

The analyzer-lsp library has a factory function:

```go
// From github.com/konveyor/analyzer-lsp/provider/lib
func GetProviderClient(config provider.Config, log logr.Logger) (provider.InternalProviderClient, error)
```

**Behavior**:
- If `config.Address != ""` → Creates **gRPC client** connecting to that address
- If `config.BinaryPath != ""` → Creates **in-process** provider
- The returned `InternalProviderClient` works the same regardless of mode

## Current Containerized Mode Analysis

### How Current Containerized Mode Works

1. **Start provider containers** (`cmd/analyze.go:1030`):
   ```go
   args := []string{fmt.Sprintf("--port=%v", init.port)}
   container.Run(..., container.WithEntrypointArgs(args...))
   // Provider listens on port 9001 inside container
   ```

2. **Provider settings** written to `settings.json` (line 1480):
   ```json
   [{
     "name": "java",
     "binaryPath": "",
     "address": "provider-xyz:9001",  // ← Container network address
     "initConfig": [...]
   }]
   ```

3. **Analyzer container** connects to providers via container network

### Problem with Current Approach

- Analyzer runs **in container**
- File I/O goes through **volume mounts** (slow on macOS)
- Provider communication is already via **network** (fast)

**Solution**: Move analyzer to host, keep provider containers!

## Proposed Hybrid Implementation

### Architecture

```
Host Process:
  ├─ Analyzer engine (direct file I/O)
  └─ Provider clients (gRPC to localhost)

Container: java-provider
  ├─ Network: host mode
  └─ Listens on: localhost:9001

Container: builtin-provider
  ├─ Network: host mode
  └─ Listens on: localhost:9002
```

### Required Changes

#### 1. Modify Provider Startup (Keep Containers)

**File**: `cmd/analyze.go` - `RunProviders()` function

**Current**:
```go
container.Run(
    ctx,
    container.WithImage(init.image),
    container.WithNetwork(networkName),  // ← Container network
    container.WithEntrypointArgs(fmt.Sprintf("--port=%v", init.port)),
    ...
)
```

**Hybrid**:
```go
container.Run(
    ctx,
    container.WithImage(init.image),
    container.WithNetwork("host"),  // ← Host network! ⭐
    container.WithEntrypointArgs(fmt.Sprintf("--port=%v", init.port)),
    ...
)
// Provider now accessible at localhost:9001
```

#### 2. Create Network Provider Configs

**File**: New function in `cmd/analyze.go`

```go
func (a *analyzeCommand) makeNetworkJavaProviderConfig(port int) provider.Config {
    return provider.Config{
        Name:       util.JavaProvider,
        BinaryPath: "",  // ← Empty for network mode
        Address:    fmt.Sprintf("localhost:%d", port),  // ← Network address! ⭐
        InitConfig: []provider.InitConfig{
            {
                Location:     a.input,  // ← Still need this for analysis
                AnalysisMode: provider.AnalysisMode(a.mode),
                ProviderSpecificConfig: map[string]interface{}{
                    // Same config as containerless
                },
            },
        },
    }
}
```

#### 3. Run Analyzer on Host (Borrow from Containerless)

**File**: `cmd/analyze.go` - New function `RunAnalysisHybrid()`

```go
func (a *analyzeCommand) RunAnalysisHybrid(ctx context.Context) error {
    // 1. Start provider containers with host network
    err := a.RunProviders(ctx, "host", volName, 5)
    if err != nil {
        return err
    }

    // 2. Create network provider configs
    providerConfigs := []provider.Config{
        a.makeNetworkJavaProviderConfig(9001),
        a.makeNetworkBuiltinProviderConfig(9002),
    }

    // 3. Create provider clients (will connect via gRPC)
    providers := map[string]provider.InternalProviderClient{}
    for _, config := range providerConfigs {
        client, err := lib.GetProviderClient(config, a.log)
        if err != nil {
            return err
        }
        providers[config.Name] = client
    }

    // 4. Initialize providers
    for name, prov := range providers {
        _, err := prov.ProviderInit(ctx, nil)
        if err != nil {
            return fmt.Errorf("failed to init %s provider: %w", name, err)
        }
    }

    // 5. Run analysis engine (borrowed from analyze-bin.go)
    eng := engine.CreateRuleEngine(ctx, 10, a.log,
        engine.WithContextLines(a.contextLines),
        engine.WithIncidentSelector(a.incidentSelector),
    )

    parser := parser.RuleParser{
        ProviderNameToClient: providers,
        Log:                  a.log.WithName("parser"),
    }

    // Parse rules and run analysis...
    // (Same as containerless mode)
}
```

## Testing Plan

### Phase 1: Verify Provider Network Mode

**Test**: Start provider container and connect from host

```bash
# Start provider container with host network
podman run -d --network=host \
  -v $(pwd)/input:/analyzer/input \
  quay.io/konveyor/jdtls-server-base:latest \
  --port=9001

# Create test config
cat > test-config.json <<EOF
[{
  "name": "java",
  "address": "localhost:9001",
  "initConfig": [{
    "location": "/path/to/code"
  }]
}]
EOF

# Test if analyzer can connect
# (Create small Go program to test lib.GetProviderClient)
```

### Phase 2: Integration Test

**Test**: Full analysis with hybrid mode

```bash
# Expected: Should work and be fast (~40s instead of 127s)
./bin/kantra analyze \
  --input pkg/testing/examples/ruleset/test-data/java \
  --output ./test-hybrid \
  --target quarkus \
  --run-local=false  # Will use hybrid mode
```

## Volume Mount Requirements

### For Providers (Still Needed)

Provider containers still need volume mounts:
- **Input source**: Providers need to read source files
- **Dependencies**: Maven cache, etc.

```go
volumes := map[string]string{
    a.input: util.SourceMountPath,
    // Provider-specific volumes
}
```

### For Analyzer (REMOVED)

Analyzer runs on host, so NO volume mounts needed:
- ✅ Reads source files directly from host
- ✅ Writes results directly to host
- ⚡ Fast I/O!

## Expected Performance Impact

### Current Benchmarks

| Mode          | Time   | CPU Usage | Bottleneck    |
|---------------|--------|-----------|---------------|
| Containerless | 34.6s  | 465%      | None (fast)   |
| Containerized | 127.9s | 0%        | Volume I/O    |

### Expected Hybrid Performance

| Mode   | Time | CPU Usage | File I/O     | Providers    |
|--------|------|-----------|--------------|--------------|
| Hybrid | ~40s | ~400%     | Direct (fast)| Containers   |

**Improvement**: 3.2x faster than current containerized mode!

**Overhead sources**:
- ~5s: Container startup time
- ~5s: Network latency vs in-process
- Still faster than 127s volume I/O!

## Open Questions & Next Steps

### Questions Answered ✅

1. **Do providers support network mode?**
   - ✅ YES - via `provider.Config.Address` field
   - ✅ Already implemented in analyzer-lsp library

2. **How does network mode work?**
   - ✅ Set `Address: "localhost:PORT"` in config
   - ✅ `lib.GetProviderClient()` creates gRPC client automatically

3. **Do we need to modify analyzer-lsp?**
   - ✅ NO - network support already exists!

### Remaining Questions ❓

1. **Do providers need volume mounts in host network mode?**
   - Need to test: Can providers access files via network share?
   - Likely YES - providers still read source files

2. **Will `--network=host` work on all platforms?**
   - macOS: Should work with Docker Desktop
   - Linux: Definitely works
   - Windows: Need to verify

3. **Provider initialization config paths**
   - Will paths in `InitConfig.Location` work correctly?
   - May need path translation between host and container

### Next Steps

1. ✅ **Research complete** - Network mode confirmed viable
2. ⏭️ **Create proof-of-concept** - Test provider connectivity
3. ⏭️ **Implement hybrid mode** - Add to kantra
4. ⏭️ **Performance benchmark** - Verify 3x improvement
5. ⏭️ **Cross-platform testing** - macOS, Linux, Windows

## References

- Provider interfaces: `github.com/konveyor/analyzer-lsp/provider`
- Provider factory: `github.com/konveyor/analyzer-lsp/provider/lib.GetProviderClient()`
- Containerless implementation: `cmd/analyze-bin.go`
- Current containerized: `cmd/analyze.go`
- Config documentation: `go doc github.com/konveyor/analyzer-lsp/provider.Config`
