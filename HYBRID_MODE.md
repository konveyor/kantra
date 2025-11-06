# Kantra Hybrid Mode

## Overview

Hybrid mode is Kantra's default analysis mode that runs the analyzer in-process while providers run in containers with network communication.

**Key Benefits:**
- ✅ 2.84x faster than containerless mode on macOS
- ✅ Clean console output with direct logging control
- ✅ Provider isolation via containers
- ✅ No local LSP server installation required
- ✅ Cross-platform compatibility

## Architecture

```
┌─────────────────────────────────────┐
│  Host (kantra binary)               │
│  ├─ Analyzer (in-process library)   │ ← Direct Go library call
│  │  └─ Direct logging control       │
│  └─ Network Provider Clients        │ ← Connect via localhost:PORT
└─────────────────────────────────────┘
         ↕ (network - localhost:PORT)
┌─────────────────────────────────────┐
│  Container (Java Provider)          │
│  └─ gRPC Service on localhost:PORT  │
└─────────────────────────────────────┘
         ↕ (network - localhost:PORT)
┌─────────────────────────────────────┐
│  Container (Go Provider)            │
│  └─ gRPC Service on localhost:PORT  │
└─────────────────────────────────────┘
```

### Why Hybrid Mode?

**Problem**: Containerized mode on macOS was 26x slower than containerless (~910s vs ~35s) due to:
- Nested containerization overhead (Podman VM + containers)
- Slow bind mount I/O performance on macOS
- Container startup and orchestration delays

**Solution**: Run the analyzer on the host (direct file I/O) while keeping providers containerized (isolation).

**Result**: ~35 seconds performance (matching containerless) with provider isolation.

## Three-Mode Strategy

Kantra supports three execution modes:

| Mode | Command | Speed | Isolation | When to Use |
|------|---------|-------|-----------|-------------|
| **Hybrid** (default) | `kantra analyze --input ...` | Fast (35s) | Providers | Production, macOS |
| **Containerless** | `kantra analyze --input ... --run-local=true` | Slower on macOS (99s) | None | Development, debugging |
| **Container** | Use override settings | Slowest (910s macOS) | Full | Legacy only |

### When to Use Each Mode

**Use Hybrid Mode (recommended):**
- ✅ Production deployments
- ✅ Running on macOS (avoids performance penalty)
- ✅ Multi-language applications
- ✅ Want clean console output
- ✅ Need provider isolation
- ✅ Don't want to install local LSP servers

**Use Containerless Mode:**
- ✅ Debugging provider behavior
- ✅ Running on Linux (good performance)
- ✅ Modifying provider code
- ✅ Testing local analyzer changes

**Use Container Mode:**
- ✅ Legacy compatibility
- ✅ Using override provider settings
- ✅ Strict security policies requiring full containerization

## Quick Start

### Basic Usage

```bash
# Hybrid mode is default - supports both source and binary analysis
kantra analyze --input /path/to/app --output ./output

# Analyze with specific target
kantra analyze --input /path/to/app --output ./output --target quarkus

# Full analysis with dependencies
kantra analyze --input /path/to/app --output ./output --mode full

# Binary analysis (WAR/JAR/EAR)
kantra analyze --input /path/to/app.war --output ./output --target quarkus
```

### Multi-Language Analysis

```bash
# Auto-detect languages (Java, Go, Python, etc.)
kantra analyze --input /path/to/mixed-app --output /tmp/output

# Force specific providers
kantra analyze --input /path/to/app --output /tmp/output --provider java --provider go
```

## How It Works

### In-Process Implementation

Hybrid mode uses the in-process analyzer library (like containerless mode) but connects to containerized providers via network:

```go
// Network provider configuration
providerConfig := provider.Config{
    Name:       "java",
    Address:    "localhost:6734",  // Network address
    BinaryPath: "",                // Empty = network mode
    InitConfig: []provider.InitConfig{ ... },
}

// Provider library automatically creates gRPC client
javaProvider := java.NewJavaProvider(log, "java", contextLines, providerConfig)
```

**Key Discovery**: The analyzer-lsp library already supports network mode when `Address` is set and `BinaryPath` is empty - no wrapper needed!

### Execution Flow

1. **Language Detection** - Uses alizer to detect languages
2. **Provider Initialization** - Allocates ports, maps providers
3. **Container Startup** - Creates volume, starts provider containers with port publishing
4. **Network Provider Setup** - Creates provider clients connecting to localhost:PORT
5. **Builtin Provider Setup** - Creates in-process builtin provider
6. **Analysis Execution** - Runs rule engine, dependency analysis
7. **Report Generation** - Generates HTML report, JSON output
8. **Cleanup** - Stops containers, removes volume

## Supported Providers

| Provider | Language | Container Image | Dependency Analysis |
|----------|----------|-----------------|---------------------|
| **Java** | Java | `konveyor/jdtls-server-base` | ✅ Maven |
| **Go** | Go | `konveyor/golang-provider` | ✅ Go modules |
| **Python** | Python | `konveyor/python-provider` | ❌ |
| **NodeJS** | JavaScript/TypeScript | `konveyor/nodejs-provider` | ❌ |
| **Dotnet** | C# | `konveyor/dotnet-provider` | ❌ |

## Troubleshooting

### Provider Fails to Start

```bash
# Check container runtime
podman version  # or: docker version

# Pull provider image manually
podman pull quay.io/konveyor/jdtls-server-base:latest

# Check running containers
podman ps -a | grep provider
```

### Provider Initialization Timeout

```bash
# Check provider container logs
podman logs <container-name>

# Verify container is running
podman ps | grep provider

# Test network connectivity
curl localhost:6734  # Should get gRPC error if running
```

### Port Conflicts

```bash
# Find process using port
lsof -i :6734

# Kill old provider containers
podman stop $(podman ps -a | grep provider | awk '{print $1}')
podman rm $(podman ps -a | grep provider | awk '{print $1}')
```

### Debug Mode

```bash
# Verbose logging
kantra analyze --input /path/to/app --output /tmp/output --verbose=5

# Check analysis.log for details
cat /tmp/output/analysis.log
```

## Performance

### Benchmark Results (macOS)

| Mode | Time | Relative |
|------|------|----------|
| **Hybrid** | ~35 seconds | 1.0x (baseline) ⚡⚡⚡ |
| **Containerless** | ~99 seconds | 2.84x slower ⚡⚡ |
| **Container** | ~910 seconds | 26x slower ⚡ |

**Why Hybrid is Faster:**
- No subprocess overhead
- Direct logging control (no filteringWriter)
- Provider isolation prevents resource conflicts
- In-process execution vs external binary

## Known Limitations

### Current Issues

1. **Health Checks** - Uses 4-second sleep instead of proper health checks

2. **Error Recovery** - Provider failure kills entire analysis, no retry logic

3. **Multi-Provider Testing** - Only Java provider tested extensively

4. **Configuration Validation** - No pre-flight checks for provider configs

### Workarounds

**If provider fails to start:**
```bash
# Use containerless mode
kantra analyze --input /path/to/app --output /tmp/output --run-local=true
```

**If analysis hangs:**
```bash
# Kill provider containers manually
podman stop $(podman ps | grep provider | awk '{print $1}')
```

## Production Readiness

**Status**: Beta - Functional but needs production hardening

### Critical TODOs

- [ ] Provider health checks (replace 4-second sleep)
- [ ] Better error messages with troubleshooting hints
- [ ] Integration tests for all provider types
- [ ] Retry logic with exponential backoff
- [ ] Graceful degradation (continue without failed providers)

### Testing Matrix

| Scenario | Status |
|----------|--------|
| Java-only hybrid | ✅ Tested |
| Go-only hybrid | ⚠️ Needs testing |
| Python-only hybrid | ⚠️ Needs testing |
| NodeJS-only hybrid | ⚠️ Needs testing |
| Multi-provider | ⚠️ Needs testing |
| Provider crash | ❌ Untested |
| Network timeout | ❌ Untested |

## Implementation Files

- **Main entry point**: `cmd/analyze.go:272` - Calls `RunAnalysisHybridInProcess()`
- **Hybrid implementation**: `cmd/analyze-hybrid.go`
  - `setupNetworkProvider()` - Generic provider setup
  - `setupBuiltinProviderHybrid()` - Builtin provider setup
  - `RunAnalysisHybridInProcess()` - Main analysis function
- **Container management**: `cmd/analyze.go:972-1021` - `RunProvidersHostNetwork()`
- **Cleanup**: `cmd/cleanup.go`

## Common Commands

```bash
# Basic analysis
kantra analyze --input /path/to/app --output /tmp/output

# With specific target
kantra analyze --input /path/to/app --output /tmp/output --target quarkus

# Custom rules
kantra analyze --input /path/to/app --output /tmp/output --rules /path/to/rules

# Multi-provider
kantra analyze --input /path/to/app --output /tmp/output --provider java --provider go

# Full analysis mode
kantra analyze --input /path/to/app --output /tmp/output --mode full

# Debug mode
kantra analyze --input /path/to/app --output /tmp/output --verbose=5

# List detected languages
kantra analyze --input /path/to/app --list-languages
```

## Summary

Hybrid mode provides the best balance of performance and isolation:
- **Fast**: Matches containerless performance (~35s)
- **Isolated**: Providers run in containers
- **Simple**: No local LSP installation required
- **Recommended**: Default for macOS and production use

---

**Last Updated**: November 2024
**Status**: Beta - Functional, production hardening in progress
**Maintainer**: Kantra Team
