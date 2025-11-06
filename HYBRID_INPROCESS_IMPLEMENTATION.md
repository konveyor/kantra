# Hybrid In-Process Implementation

## Overview

This document describes the new hybrid mode architecture that runs the analyzer in-process (like containerless mode) while providers run in containers, using network communication instead of external binary execution.

## Architecture

**Previous Hybrid Approach (External Binary):**
```
┌─────────────────────────────────────┐
│  Host (kantra)                      │
│  ├─ External analyzer binary        │ ← Executed as subprocess
│  │  └─ Writes to file/stdout        │
│  └─ filteringWriter                 │ ← Filters binary output
└─────────────────────────────────────┘
         ↕ (network)
┌─────────────────────────────────────┐
│  Container (Java Provider)          │
│  └─ gRPC Service on localhost:PORT  │
└─────────────────────────────────────┘
```

**New Hybrid Approach (In-Process):**
```
┌─────────────────────────────────────┐
│  Host (kantra)                      │
│  ├─ Analyzer (in-process library)   │ ← Direct Go library call
│  │  └─ Direct logging control       │
│  └─ Network Provider Clients        │ ← Connect via localhost:PORT
└─────────────────────────────────────┘
         ↕ (network)
┌─────────────────────────────────────┐
│  Container (Java Provider)          │
│  └─ gRPC Service on localhost:PORT  │
└─────────────────────────────────────┘
```

## Key Benefits

1. **Clean Output:** Same clean console output as containerless mode (no filteringWriter needed)
2. **Direct Control:** In-process execution allows direct logging and progress control
3. **Provider Isolation:** Providers still run in containers for consistency and isolation
4. **Performance:** Avoids subprocess overhead and complex output filtering
5. **Code Reuse:** Uses same code path as containerless mode for analyzer execution

## Implementation Details

### Network Provider Configuration

The key discovery is that `java.NewJavaProvider()` already supports network mode via configuration:

```go
javaConfig := provider.Config{
    Name:       util.JavaProvider,
    Address:    fmt.Sprintf("localhost:%d", provInit.port), // Connect to container
    BinaryPath: "",                                          // Empty = network mode
    InitConfig: []provider.InitConfig{
        {
            Location:     a.input,
            AnalysisMode: provider.AnalysisMode(a.mode),
            ProviderSpecificConfig: map[string]interface{}{
                "lspServerName": util.JavaProvider,
            },
        },
    },
}

// This automatically creates a network client (no wrapper needed!)
javaProvider := java.NewJavaProvider(analysisLog, "java", a.contextLines, javaConfig)
```

**No wrapper class needed!** The provider library already has built-in support for network mode when:
- `Address` field is set to `localhost:PORT`
- `BinaryPath` field is empty

### File Structure

#### New File: `cmd/analyze-hybrid.go`

Contains helper functions for hybrid mode:

1. **`setupJavaProviderHybrid()`** - Creates network-based Java provider client
2. **`setupBuiltinProviderHybrid()`** - Creates in-process builtin provider
3. **`RunAnalysisHybridInProcess()`** - Main analysis function using in-process analyzer

#### Modified File: `cmd/analyze.go`

Changed line 273 to call the new in-process function:

```go
// Before:
err = analyzeCmd.RunAnalysisHybrid(cmdCtx)

// After:
err = analyzeCmd.RunAnalysisHybridInProcess(cmdCtx)
```

### Execution Flow

1. **Start containerized providers:**
   - Create volume for source code
   - Start provider containers with port publishing (`-p PORT:PORT`)
   - Wait for providers to initialize (4 seconds, TODO: health checks)

2. **Setup provider clients:**
   - Create network-based Java provider client (connects to `localhost:PORT`)
   - Create in-process builtin provider

3. **Run analyzer in-process:**
   - Setup logging (file + console hook, like containerless)
   - Create rule engine
   - Parse rules
   - Execute analysis
   - Run dependency analysis (if full mode)
   - Write results

4. **Generate outputs:**
   - Write `output.yaml`
   - Create JSON output (if requested)
   - Generate static report

### Logging Strategy

Same as containerless mode:

```go
// Analyzer logs go to file
logrusAnalyzerLog := logrus.New()
logrusAnalyzerLog.SetOutput(analysisLog)

// Console hook prints rule processing messages to console
consoleHook := &ConsoleHook{Level: logrus.InfoLevel, Log: a.log}
logrusAnalyzerLog.AddHook(consoleHook)

// Error logs go to stderr
logrusErrLog := logrus.New()
logrusErrLog.SetOutput(os.Stderr)
```

This provides clean console output with important messages while detailed logs go to `analysis.log`.

## POC Evidence

File: `/Users/tsanders/Workspace/kantra/poc-files/test-java-provider-network.go`

This POC demonstrated that the provider library already supports network mode:

```go
testConfig := provider.Config{
    Name:       "java",
    BinaryPath: "",                // Empty = network mode
    Address:    "localhost:9001",  // Network address
    InitConfig: []provider.InitConfig{
        {
            Location:     "/tmp",
            AnalysisMode: provider.FullAnalysisMode,
        },
    },
}

// This automatically creates a network client!
client := java.NewJavaProvider(logger, "java", 10, testConfig)
```

The POC confirmed that no custom wrapper implementation was needed - the existing code already handles network communication.

## Comparison with Containerless Mode

### Similarities
- In-process analyzer execution
- Direct logging control
- Same rule engine, parser, and output code
- Clean console output

### Differences
- **Providers:** Containerless runs Java provider in-process; hybrid runs in container
- **Configuration:** Containerless uses `BinaryPath` for jdtls; hybrid uses `Address` for network
- **Requirements:** Containerless needs local jdtls/maven; hybrid only needs container runtime
- **Dependencies:** Hybrid needs default rulesets extracted from container

## TODO

1. **Health Checks:** Replace 4-second sleep with proper provider health checks
2. **Error Handling:** Improve provider connection error messages
3. **Testing:** Add integration tests for network provider mode
4. **Cleanup:** Consider deprecating old `RunAnalysisHybrid()` external binary approach
5. **Documentation:** Update user docs to explain hybrid mode architecture

## Testing

To test the implementation:

```bash
# Build kantra
go build -o kantra

# Run hybrid mode analysis (--run-local=false is default)
./kantra analyze --input /path/to/app --output /tmp/output
```

Expected output should be clean and similar to containerless mode, with providers running in containers.

## References

- Original hybrid mode: `cmd/analyze.go` lines 1168-1296 (RunAnalysisHybrid)
- Containerless mode: `cmd/analyze-bin.go` lines 62-256 (RunAnalysisContainerless)
- Network provider POC: `poc-files/test-java-provider-network.go`
- New hybrid implementation: `cmd/analyze-hybrid.go`
