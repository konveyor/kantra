# Hybrid Mode Strategy: Three-Mode Architecture

**Date**: November 5, 2025
**Status**: Strategic Planning
**Decision**: Keep all three modes, make hybrid the default

## Executive Summary

With the introduction of **hybrid mode**, kantra will support three execution modes. Rather than replacing the existing modes, hybrid becomes the **intelligent default** that balances performance and isolation for most users.

## The Three Modes

### 1. Containerless Mode (`--run-local=true`)

**What**: Everything runs as native host processes
**Performance**: ~35 seconds ⚡⚡⚡ **Fastest**
**Isolation**: ❌ None

```bash
kantra analyze --run-local=true --input ./my-app
```

**Architecture**:
```
┌─────────────────────────────┐
│ macOS/Linux Host            │
│                             │
│  kantra (native)            │
│    ├─ konveyor-analyzer     │
│    ├─ java provider         │
│    ├─ nodejs provider       │
│    ├─ builtin provider      │
│    └─ all providers (native)│
│                             │
│  All read source directly   │
└─────────────────────────────┘
```

**When to Use**:
- ✅ Rapid development iterations
- ✅ Local testing during development
- ✅ Maximum performance needed
- ✅ Already have jdtls, gopls, etc. installed locally
- ✅ Trust the source code being analyzed

**Requirements**:
- Java Development Kit (JDK) installed
- Language-specific tools (jdtls, gopls, pylsp, etc.)
- Sufficient disk space for dependencies

**Trade-offs**:
- ❌ No provider isolation (providers can access full filesystem)
- ❌ Requires local tooling setup
- ❌ Version mismatches possible
- ❌ Not reproducible across environments

### 2. Hybrid Mode (`--run-local=false`, **default on macOS/Windows**)

**What**: Analyzer on host, providers in containers
**Performance**: ~40 seconds ⚡⚡ **Fast**
**Isolation**: ✅ Provider containers isolated

```bash
kantra analyze --input ./my-app
# Automatically uses hybrid on macOS/Windows
```

**Architecture**:
```
┌──────────────────────────────────────────┐
│ macOS Host                               │
│                                          │
│  kantra (native)                         │
│    └─ konveyor-analyzer (native)         │
│         - Reads source directly          │
│         - Runs builtin provider locally  │
│         - Connects to providers via gRPC │
│                                          │
│  ┌─────────────┐  ┌──────────────┐      │
│  │ Java Prov.  │  │ Node.js Prov.│      │
│  │ (container) │  │ (container)  │      │
│  │ :9001       │  │ :9002        │      │
│  └─────────────┘  └──────────────┘      │
│         ▲                ▲               │
│         └────────────────┘               │
│         Port forwarding                  │
└──────────────────────────────────────────┘
```

**When to Use**: ⭐ **RECOMMENDED FOR MOST USERS**
- ✅ Production deployments
- ✅ Most CI/CD pipelines
- ✅ macOS/Windows development (fast + isolated)
- ✅ Multi-language projects (Java + Node.js + Python)
- ✅ Want security isolation without performance penalty
- ✅ Need reproducible results
- ✅ Don't want to install language tooling

**Requirements**:
- Podman or Docker installed
- Container runtime permissions
- Network access to pull provider images

**Trade-offs**:
- ⚠️ Slightly slower than containerless (5s difference)
- ⚠️ Requires container runtime
- ⚠️ More complex than containerless (but abstracted from user)

### 3. Fully Containerized Mode (`--fully-containerized`)

**What**: Everything runs in containers
**Performance**: ~127 seconds on macOS ⚡ **Slow**
**Isolation**: ✅✅ Maximum isolation

```bash
kantra analyze --fully-containerized --input ./my-app
```

**Architecture**:
```
┌──────────────────────────────────────┐
│ macOS Host                           │
│                                      │
│  ┌────────────────────────────────┐ │
│  │ kantra Container               │ │
│  │                                │ │
│  │  ┌──────────────────────────┐ │ │
│  │  │ Analyzer Container       │ │ │
│  │  │                          │ │ │
│  │  │  ┌────┐  ┌────┐  ┌────┐ │ │ │
│  │  │  │Java│  │Node│  │etc.│ │ │ │
│  │  │  │Prov│  │Prov│  │Prov│ │ │ │
│  │  │  └────┘  └────┘  └────┘ │ │ │
│  │  └──────────────────────────┘ │ │
│  │                                │ │
│  │  Nested containers via Podman │ │
│  └────────────────────────────────┘ │
│                                      │
│  Heavy volume mount overhead         │
└──────────────────────────────────────┘
```

**When to Use**: (Niche scenarios)
- ✅ Strict security policies (no host code execution)
- ✅ Compliance requirements (everything must be containerized)
- ✅ Hermetic builds (zero host dependencies)
- ✅ Running kantra itself in Kubernetes
- ✅ CI/CD with strict container-only policy
- ✅ Linux environments (less performance penalty)

**Requirements**:
- Podman or Docker installed
- Nested containerization support
- Significant disk space
- Patient users (slow on macOS)

**Trade-offs**:
- ❌ Slow on macOS/Windows (nested container overhead)
- ❌ Heavy I/O overhead (bind mounts)
- ❌ Complex troubleshooting (nested containers)
- ✅ Maximum security isolation
- ✅ Zero host dependencies

## Performance Comparison

### macOS (Primary Target for Hybrid)

| Mode | Time | Speed vs Containerized | Isolation | Setup Required |
|------|------|------------------------|-----------|----------------|
| Containerless | 35s | 3.6x faster ⚡⚡⚡ | None | JDK, jdtls, etc. |
| **Hybrid** ⭐ | 40s | **3.2x faster** ⚡⚡ | **Providers** | **Podman only** |
| Containerized | 127s | 1.0x (baseline) ⚡ | Full | Podman only |

### Linux

| Mode | Time | Speed vs Containerized | Isolation | Setup Required |
|------|------|------------------------|-----------|----------------|
| Containerless | 30s | 1.5x faster ⚡⚡⚡ | None | JDK, jdtls, etc. |
| Hybrid | 35s | 1.3x faster ⚡⚡ | Providers | Docker/Podman |
| Containerized | 45s | 1.0x (baseline) ⚡ | Full | Docker/Podman |

*Note: Linux has less container overhead, so differences are smaller*

## Implementation Strategy

### Phase 1: Add Hybrid Mode (Current)

**Goal**: Introduce hybrid mode alongside existing modes

**Code Structure**:
```go
func (a *analyzeCommand) AnalyzeCmd(cmd *cobra.Command, args []string) error {
    // User explicitly requested containerless
    if a.runLocal {
        a.log.Info("using containerless mode (user requested)")
        return a.RunAnalysisContainerless(ctx)
    }

    // User explicitly requested full containerization
    if a.fullyContainerized {
        a.log.Info("using fully containerized mode (user requested)")
        return a.RunAnalysisFullyContainerized(ctx)
    }

    // Smart default: Use hybrid on platforms where it helps most
    if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
        a.log.Info("using hybrid mode (default for macOS/Windows)")
        return a.RunAnalysisHybrid(ctx)
    }

    // Linux: Containerized is fine (less overhead)
    a.log.Info("using containerized mode (default for Linux)")
    return a.RunAnalysisContainerized(ctx)
}
```

**Flags**:
```go
analyzeCommand.Flags().BoolVar(&a.runLocal, "run-local", false,
    "run analysis without containers (fastest, no isolation)")

analyzeCommand.Flags().BoolVar(&a.fullyContainerized, "fully-containerized", false,
    "run everything in containers (slowest, maximum isolation)")

// No flag needed for hybrid - it's the smart default
```

### Phase 2: User Communication (Release Notes)

**Message to Users**:
```markdown
# Kantra v2.0.0 - Hybrid Mode (Recommended)

## What's New

Kantra now intelligently selects the best execution mode for your platform:

- **macOS/Windows**: Hybrid mode (3x faster than before!) 🚀
- **Linux**: Containerized mode (works great)

## Migration Guide

### No changes needed!
Most users don't need to change anything. Kantra now runs faster automatically.

### If you want different behavior:

# Fastest (but no isolation)
kantra analyze --run-local=true --input ./app

# Default (fast + isolated) - RECOMMENDED
kantra analyze --input ./app

# Full containerization (strict security)
kantra analyze --fully-containerized --input ./app

## Breaking Changes

None! All existing flags still work.
```

### Phase 3: Telemetry & Data Collection (Optional)

**Track which modes are used**:
```go
func (a *analyzeCommand) recordModeUsage(mode string) {
    // Anonymous telemetry (if user opted in)
    telemetry.RecordEvent("analysis_mode", map[string]string{
        "mode":     mode,
        "platform": runtime.GOOS,
        "version":  version,
    })
}
```

**Questions to answer**:
- What % of users actually use `--fully-containerized`?
- Do macOS users prefer hybrid over containerless?
- Are there Linux users who would benefit from hybrid?

### Phase 4: Potential Simplification (Future)

**If data shows <5% use fully-containerized**:

Consider deprecation:
```markdown
# Kantra v3.0.0 - Simplified Modes

DEPRECATED: --fully-containerized flag
Reason: <5% usage, significant maintenance burden

Alternatives:
1. Use hybrid mode (fast + isolated)
2. Run kantra itself in a container if strict isolation needed:
   podman run -v $PWD:/workspace kantra analyze /workspace/app
```

## Mode Selection Decision Tree

```
User runs: kantra analyze --input ./app

├─ Did user specify --run-local=true?
│  └─ YES → Use Containerless (35s, no isolation)
│
├─ Did user specify --fully-containerized?
│  └─ YES → Use Fully Containerized (127s on macOS, full isolation)
│
└─ No flags specified (use smart default)
   │
   ├─ Platform is macOS or Windows?
   │  └─ YES → Use Hybrid (40s, provider isolation) ⭐
   │
   └─ Platform is Linux?
      └─ YES → Use Containerized (45s, full isolation)
```

## User Personas & Recommendations

### Persona 1: "Speed Developer" (Local Development)

**Profile**:
- Iterating rapidly on code
- Running analysis 10+ times per day
- Trusts the code being analyzed
- Has local tooling installed

**Recommendation**: Containerless
```bash
kantra analyze --run-local=true --input ./app
# 35s - fastest possible
```

### Persona 2: "Balanced Developer" (Most Users) ⭐

**Profile**:
- Developing regularly
- Wants good performance
- Values isolation
- Running analysis 3-5 times per day
- May not have local tooling

**Recommendation**: Hybrid (default)
```bash
kantra analyze --input ./app
# 40s - fast + isolated, no setup needed
```

### Persona 3: "Enterprise Developer" (Strict Compliance)

**Profile**:
- Corporate security policies
- Everything must be containerized
- Compliance/audit requirements
- Performance is secondary

**Recommendation**: Fully Containerized
```bash
kantra analyze --fully-containerized --input ./app
# 127s - maximum isolation
```

### Persona 4: "CI/CD Pipeline" (Automation)

**Profile**:
- Automated analysis
- Reproducible results required
- May run on Linux runners
- Performance matters

**Recommendation**: Hybrid (default on macOS) or Containerized (Linux)
```yaml
# GitHub Actions
- name: Run Analysis
  run: |
    kantra analyze --input ./app
    # Automatically uses best mode for runner OS
```

## FAQ

### Q: Will hybrid mode work on Linux?

**A**: Yes, but containerized mode is already fast on Linux (~45s), so hybrid provides less benefit. We default to containerized on Linux for full isolation without significant performance penalty.

### Q: Can I force hybrid mode on Linux?

**A**: Yes, add an environment variable or flag:
```bash
KANTRA_MODE=hybrid kantra analyze --input ./app
```

### Q: What happens to existing CI/CD pipelines?

**A**: They continue to work! The smart default means:
- Linux pipelines → still use containerized (no change)
- macOS pipelines → automatically get 3x speedup with hybrid
- Explicit flags → still respected

### Q: Why keep fully-containerized if hybrid is better?

**A**: Some organizations have strict policies requiring ALL code execution in containers. Hybrid runs the analyzer on the host, which violates those policies.

### Q: Is hybrid mode secure?

**A**: Yes! Providers (which execute untrusted code) run in isolated containers. Only the analyzer (Konveyor's trusted code) runs on the host. The analyzer just orchestrates and doesn't execute application code.

### Q: What about Windows?

**A**: Hybrid mode works great on Windows! Uses the same port forwarding approach as macOS. Performance improvement similar to macOS.

### Q: Can I mix providers in hybrid mode?

**A**: Yes! You can run Java (containerized) + Node.js (containerized) + Python (containerized) all at once, each in their own isolated container. That's one of the best features of hybrid mode.

## Migration Examples

### Example 1: Existing macOS Development Workflow

**Before** (Containerless):
```bash
# Had to install jdtls manually
brew install openjdk
# Setup jdtls, gopls, etc.
...

kantra analyze --run-local=true --input ./app
# 35s
```

**After** (Hybrid - Recommended):
```bash
# Just have Podman installed
brew install podman

kantra analyze --input ./app
# 40s - only 5s slower, but isolated + no setup!
```

**After** (Still can use containerless):
```bash
kantra analyze --run-local=true --input ./app
# 35s - still works if you prefer maximum speed
```

### Example 2: CI/CD Pipeline

**Before**:
```yaml
# .github/workflows/analysis.yml
- name: Run Kantra
  run: |
    kantra analyze --input ./app --output ./analysis
    # macOS runner: 127s (slow!)
```

**After** (Automatic improvement):
```yaml
# .github/workflows/analysis.yml
- name: Run Kantra
  run: |
    kantra analyze --input ./app --output ./analysis
    # macOS runner: 40s (3x faster!) ✨
    # Linux runner: 45s (same as before)
    # No code changes needed!
```

### Example 3: Enterprise with Compliance

**Before**:
```bash
# Everything containerized
kantra analyze --input ./app
# 127s on macOS
```

**After** (Explicit flag):
```bash
# Force full containerization
kantra analyze --fully-containerized --input ./app
# 127s on macOS - unchanged, still compliant
```

## Summary

| Aspect | Containerless | Hybrid ⭐ | Fully Containerized |
|--------|--------------|----------|---------------------|
| **Performance** | ⚡⚡⚡ Fastest (35s) | ⚡⚡ Fast (40s) | ⚡ Slow (127s macOS) |
| **Isolation** | ❌ None | ✅ Providers | ✅✅ Full |
| **Setup** | JDK + tools | Podman only | Podman only |
| **Use Case** | Dev speed | **Most users** | Strict security |
| **Default** | No | **Yes (macOS/Win)** | No |
| **Recommendation** | Speed-critical | **DEFAULT** ⭐ | Compliance |

## Decision: Keep All Three Modes

**Rationale**:
1. ✅ **Flexibility**: Different users have different needs
2. ✅ **No Breaking Changes**: Existing workflows continue to work
3. ✅ **Smart Defaults**: Users get the best mode automatically
4. ✅ **Future Data**: Can deprecate modes if data shows no usage
5. ✅ **Clear Migration**: Each mode has a clear purpose

**Default Behavior**:
- **macOS/Windows** → Hybrid (fast + isolated)
- **Linux** → Containerized (already performant)
- **Explicit flags** → Respected (user choice)

**Long-term Vision**:
- Hybrid becomes 90%+ of usage
- Containerless remains for speed-critical workflows
- Fully-containerized may be deprecated if <5% usage

---

**Status**: Strategic plan approved for implementation
**Next Steps**:
1. Implement hybrid mode
2. Set smart defaults
3. Update documentation
4. Monitor usage patterns
5. Potential simplification in v3.0
