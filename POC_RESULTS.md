# POC Results: Provider Network Connectivity

## Executive Summary

**✅ SUCCESS**: Provider containers can be started with `--network=host` and listen on `localhost:PORT`

**⚠️ BLOCKER**: Direct use of `lib.GetProviderClient()` with network address hangs - needs investigation

**📝 NEXT STEP**: Investigate analyzer-lsp library's network provider support or consider alternative approaches

## What We Tested

### Test Setup

1. Started Java provider container with host network:
   ```bash
   podman run -d \
     --name poc-java-provider \
     --network=host \
     -v ./input:/analyzer/input:ro \
     quay.io/konveyor/java-external-provider:latest \
     --port=9001
   ```

2. Provider started successfully:
   ```
   ✅ Container started
   ✅ Listening on [::]:9001
   ```

3. Attempted to connect from host using Go program:
   ```go
   config := provider.Config{
       Name: "java",
       Address: "localhost:9001",  // Network mode
       BinaryPath: "",              // No binary = network
   }
   client, err := lib.GetProviderClient(config, logger)
   ```

### Results

| Component | Status | Notes |
|-----------|--------|-------|
| Provider container startup | ✅ PASS | Container runs and listens on localhost:9001 |
| Host network mode | ✅ PASS | --network=host works on macOS/Podman |
| lib.GetProviderClient() | ❌ HANGS | Function doesn't return when Address is set |

## Analysis

### Current Architecture

**Containerized Mode**:
```
Host (kantra CLI)
  └─> Analyzer Container
      ├─> Reads settings.json (has Address fields)
      ├─> Connects to provider containers via network
      └─> konveyor-analyzer binary handles connection
```

**Containerless Mode**:
```
Host (kantra CLI)
  └─> Analyzer Library (engine.CreateRuleEngine)
      ├─> lib.GetProviderClient(config with BinaryPath)
      ├─> Providers run in-process
      └─> Direct Go function calls
```

### The Problem

1. **Konveyor-analyzer binary** (used in containerized mode):
   - Can connect to network providers via settings.json
   - **Not available on host** - only in container image
   - Can't use for hybrid approach

2. **lib.GetProviderClient()** (used in containerless mode):
   - Works for in-process providers (BinaryPath set)
   - **Hangs when using network mode** (Address set, BinaryPath empty)
   - Unclear if network mode is fully implemented for this use case

## Possible Causes for Hang

### Theory 1: Network Mode Not Fully Supported
`lib.GetProviderClient()` may not implement network connectivity when called directly from Go code. It might assume it's being called from the konveyor-analyzer binary.

### Theory 2: Missing Configuration
Network mode might require additional configuration we're not providing (TLS certs, auth tokens, etc.).

### Theory 3: gRPC Client Not Ready
The function might be trying to establish a connection immediately and timing out because the provider needs more setup time.

## Next Steps

### Option 1: Deep Dive into analyzer-lsp Library ⭐ RECOMMENDED

**Goal**: Understand how lib.GetProviderClient() works with network providers

**Tasks**:
1. Clone analyzer-lsp repository
2. Read lib.GetProviderClient() implementation
3. Find network provider client code
4. Understand what's needed for network mode
5. Fix/update our usage

**Pros**: Proper solution, uses library as intended
**Cons**: Requires understanding analyzer-lsp internals

### Option 2: Extract konveyor-analyzer Binary

**Goal**: Get the konveyor-analyzer binary onto the host

**Tasks**:
1. Extract binary from container image
2. Install dependencies (if any)
3. Run analyzer binary on host with settings.json
4. Point to localhost provider addresses

**Command**:
```bash
# Extract binary from container
podman cp analyzer:/usr/local/bin/konveyor-analyzer ./

# Run with network provider settings
./konveyor-analyzer --provider-settings=./settings.json ...
```

**Pros**: Known to work (containerized mode uses this)
**Cons**: Adds binary dependency, may need container libs

### Option 3: Implement Custom gRPC Client

**Goal**: Write our own gRPC client to connect to providers

**Tasks**:
1. Find provider gRPC proto definitions
2. Generate Go client code
3. Implement provider.InternalProviderClient interface
4. Use in place of lib.GetProviderClient()

**Pros**: Full control over network communication
**Cons**: Lot of work, duplicates analyzer-lsp code

### Option 4: Hybrid Container Approach

**Goal**: Keep analyzer in container but optimize volume mounts

**Tasks**:
1. Use named volumes instead of bind mounts
2. Add `:cached` or `:delegated` flags
3. Consider tmpfs for intermediate files

**Pros**: Simpler, less invasive
**Cons**: May not eliminate all volume overhead

## Recommended Path Forward

**Immediate (Next 1 hour)**:
1. ✅ **Option 2**: Extract konveyor-analyzer binary from container
2. ✅ Test if it can connect to localhost providers
3. ✅ If it works, proceed with hybrid implementation using binary

**Short Term (If binary approach works)**:
1. Implement hybrid mode using extracted binary
2. Benchmark performance
3. Document as working POC

**Long Term (Future enhancement)**:
1. Work with analyzer-lsp maintainers to clarify network mode support
2. Contribute fixes if needed
3. Replace binary approach with proper library usage

## Files Created

- `poc-test-provider.go` - Go program to test network connectivity
- `poc-start-provider.sh` - Script to start provider with host network
- `POC_RESULTS.md` - This file

## Conclusion

Provider containers **CAN** run with host network and listen on localhost. The blocker is how to connect to them from the host. The most pragmatic next step is to try extracting and using the konveyor-analyzer binary on the host, since we know it already supports network providers.

If the binary approach works, we can proceed with hybrid implementation and achieve the 3x performance improvement. Library-based approach can be pursued as a future enhancement after consulting with analyzer-lsp team.
