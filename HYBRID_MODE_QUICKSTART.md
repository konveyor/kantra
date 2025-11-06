# Hybrid Mode Quick Reference

Quick reference guide for Kantra's hybrid analysis mode.

## TL;DR

Hybrid mode runs the analyzer on your host while providers run in containers. It's the default mode and recommended for most use cases.

```bash
# Just run it - hybrid mode is default
kantra analyze --input /path/to/app --output /tmp/output
```

## Quick Comparison

| Mode | Command | Speed | When to Use |
|------|---------|-------|-------------|
| **Hybrid** (default) | `kantra analyze --input ... --output ...` | Fast | Production, macOS |
| **Containerless** | `kantra analyze --input ... --output ... --run-local=true` | Slower on macOS | Development, debugging |
| **Container** | Use override settings | Slowest | Legacy only |

## Common Commands

### Basic Analysis
```bash
# Analyze Java app
kantra analyze --input /path/to/java-app --output /tmp/output

# Analyze with specific target
kantra analyze --input /path/to/app --output /tmp/output --target quarkus

# Analyze with custom rules
kantra analyze --input /path/to/app --output /tmp/output --rules /path/to/rules
```

### Multi-Language Apps
```bash
# Auto-detect languages (Java, Go, Python, etc.)
kantra analyze --input /path/to/mixed-app --output /tmp/output

# Force specific providers
kantra analyze --input /path/to/app --output /tmp/output --provider java --provider go
```

### Analysis Modes
```bash
# Full analysis (source + dependencies)
kantra analyze --input /path/to/app --output /tmp/output --mode full

# Source-only (no dependency analysis)
kantra analyze --input /path/to/app --output /tmp/output --mode source-only
```

### Debugging
```bash
# Verbose logging
kantra analyze --input /path/to/app --output /tmp/output --verbose=5

# Check detected languages
kantra analyze --input /path/to/app --list-languages

# List available providers
kantra analyze --list-providers
```

## Quick Troubleshooting

### Provider Won't Start

**Check container runtime:**
```bash
podman version  # or: docker version
```

**Pull provider images manually:**
```bash
podman pull quay.io/konveyor/jdtls-server-base:latest
```

**Check running containers:**
```bash
podman ps -a | grep provider
```

### Analysis Hangs

**Kill stuck providers:**
```bash
podman stop $(podman ps | grep provider | awk '{print $1}')
```

**Check provider logs:**
```bash
podman logs <container-name>
```

### Port Conflicts

**Find what's using the port:**
```bash
lsof -i :6734
```

**Clean up old containers:**
```bash
podman rm -f $(podman ps -a | grep provider | awk '{print $1}')
```

### Volume Mount Issues (macOS)

**Start Podman machine:**
```bash
podman machine start
```

**Check Podman machine:**
```bash
podman machine list
```

## What Gets Created

```
/tmp/output/
├── output.yaml           # Analysis results
├── dependencies.yaml     # Dependency analysis (full mode only)
├── analysis.log          # Detailed logs
├── static-report/        # HTML report
│   └── index.html
└── .rulesets/            # Cached default rulesets (hybrid mode)
```

## Output Files

**Main results:**
- `output.yaml` - Violations found by analysis
- `dependencies.yaml` - Dependencies (full mode only)
- `static-report/index.html` - Browse-able HTML report

**Logs:**
- `analysis.log` - Detailed analyzer logs
- Console output - Progress and important messages

## Architecture (Simple)

```
Your Computer (Host)
├── kantra (analyzer runs here)
│   ├── Reads your code directly
│   └── Connects to providers via network
│
└── Containers (providers run here)
    ├── Java Provider (port 6734)
    ├── Go Provider (port 6735)
    └── Python Provider (port 6736)
```

**Benefits:**
- Fast (no subprocess overhead)
- Clean output (no filtering needed)
- Cross-platform (works on macOS)
- Isolated providers (consistent results)

## Supported Languages

| Language | Provider | Container Image | Dependency Analysis |
|----------|----------|-----------------|---------------------|
| Java | java | `konveyor/jdtls-server-base` | ✅ |
| Go | go | `konveyor/golang-provider` | ✅ |
| Python | python | `konveyor/python-provider` | ❌ |
| JavaScript/TypeScript | nodejs | `konveyor/nodejs-provider` | ❌ |
| C# | dotnet | `konveyor/dotnet-provider` | ❌ |

## Environment Variables

```bash
# Proxy settings (auto-detected)
export HTTP_PROXY=http://proxy.example.com:8080
export HTTPS_PROXY=https://proxy.example.com:8443
export NO_PROXY=localhost,127.0.0.1

# Or pass as flags
kantra analyze --input /path/to/app --output /tmp/output \
  --http-proxy http://proxy.example.com:8080 \
  --https-proxy https://proxy.example.com:8443
```

## Common Flags

```bash
# Input/Output
--input, -i       Path to application source code
--output, -o      Path to output directory

# Sources/Targets
--source, -s      Source technology (e.g., eap6, eap7)
--target, -t      Target technology (e.g., quarkus, cloud-readiness)

# Rules
--rules           Custom rule files or directories
--label-selector  Run rules based on label selector

# Analysis
--mode            Analysis mode: full or source-only (default: full)
--provider        Force specific provider(s)

# Configuration
--maven-settings  Path to custom Maven settings.xml
--context-lines   Lines of code context (default: 100)

# Debugging
--verbose         Log level (0-9, default: 0)
--list-languages  Detect and list languages
--list-providers  List available providers
--list-sources    List available source labels
--list-targets    List available target labels

# Mode Selection
--run-local       Use containerless mode (default: false)
```

## Examples

### Migrate EAP to Quarkus
```bash
kantra analyze \
  --input /path/to/eap-app \
  --output /tmp/output \
  --source eap7 \
  --target quarkus
```

### Cloud Readiness Assessment
```bash
kantra analyze \
  --input /path/to/app \
  --output /tmp/output \
  --target cloud-readiness
```

### Custom Rules Only
```bash
kantra analyze \
  --input /path/to/app \
  --output /tmp/output \
  --rules /path/to/custom-rules \
  --enable-default-rulesets=false
```

### Binary Analysis (JAR/WAR/EAR)
```bash
kantra analyze \
  --input /path/to/app.war \
  --output /tmp/output \
  --target quarkus
```

### With Maven Settings
```bash
kantra analyze \
  --input /path/to/app \
  --output /tmp/output \
  --maven-settings ~/.m2/settings.xml
```

## Tips & Tricks

### Speed Up Repeated Analyses

Default rulesets are cached in `{output}/.rulesets/` on first run. Reuse the same output directory:

```bash
# First run (extracts rulesets)
kantra analyze --input /path/to/app --output /tmp/output

# Subsequent runs (uses cached rulesets)
kantra analyze --input /path/to/app --output /tmp/output --overwrite
```

### Check What Will Run

```bash
# See detected languages
kantra analyze --input /path/to/app --list-languages

# See available migration paths
kantra analyze --list-sources
kantra analyze --list-targets
```

### Reduce Output Size

```bash
# Fewer context lines
kantra analyze --input /path/to/app --output /tmp/output --context-lines=10

# Skip static report
kantra analyze --input /path/to/app --output /tmp/output --skip-static-report

# Source-only mode (no dependencies)
kantra analyze --input /path/to/app --output /tmp/output --mode source-only
```

### Filter Results

```bash
# Exclude known libraries
kantra analyze --input /path/to/app --output /tmp/output --analyze-known-libraries=false

# Custom incident selector
kantra analyze --input /path/to/app --output /tmp/output \
  --incident-selector '(!package=io.konveyor.demo.config-utils)'
```

## Performance Tips

1. **Use hybrid mode on macOS** (default) - 2.84x faster than containerless
2. **Reuse output directory** - Caches rulesets and Maven dependencies
3. **Use source-only mode** if you don't need dependency analysis
4. **Reduce context-lines** if output size is too large
5. **Use label-selector** to run only specific rules

## Need More Help?

- **Full documentation**: [HYBRID_MODE.md](./HYBRID_MODE.md)
- **Architecture details**: [HYBRID_INPROCESS_IMPLEMENTATION.md](./HYBRID_INPROCESS_IMPLEMENTATION.md)
- **Mode strategy**: [HYBRID_MODE_STRATEGY.md](./HYBRID_MODE_STRATEGY.md)
- **GitHub Issues**: https://github.com/konveyor/kantra/issues

## Status

**Current**: Beta - Functional but some rough edges
**Recommended**: Yes, for macOS and multi-language apps
**Tested**: Java provider extensively, others need more testing

---

**Quick Links:**
- [Full Hybrid Mode Docs](./HYBRID_MODE.md)
- [Troubleshooting](./HYBRID_MODE.md#troubleshooting)
- [Architecture](./HYBRID_MODE.md#architecture)
- [Production Readiness](./HYBRID_MODE.md#production-readiness)
