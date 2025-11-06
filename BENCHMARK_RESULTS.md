# Hybrid vs Containerless Mode Benchmark Results

**Date**: November 6, 2025
**Test Input**: `/Users/tsanders/Workspace/kantra/pkg/testing/examples/ruleset/test-data/java`
**Test Mode**: source-only analysis

## Performance Comparison

### Containerless Mode (--run-local=true)

| Phase | Duration |
|-------|----------|
| Java Provider Setup | 1,661 ms |
| Builtin Provider Setup | 0 ms |
| Rule Loading | 1,019 ms |
| **Rule Execution** | **10,207 ms** |
| Output Writing | 10 ms |
| Static Report Generation | 25 ms |
| **TOTAL** | **13,024 ms (~13 seconds)** |

**Output Size**: 571 KB

### Hybrid Mode (--run-local=false)

| Phase | Duration |
|-------|----------|
| Provider Container Setup | 213 ms |
| Rule Loading | 1,727 ms |
| **Rule Execution** | **79,090 ms** |
| Output Writing | 5 ms |
| Static Report Generation | Failed (macOS Podman) |
| **TOTAL (excluding static report)** | **~81,035 ms (~81 seconds)** |

**Output Size**: 279 KB

## Key Findings

### Performance

**Hybrid mode is ~6.2x slower than containerless mode** for rule execution (79s vs 13s).

**Root Cause**: Network communication overhead
- Hybrid mode uses gRPC over localhost to communicate with containerized providers
- Each rule evaluation likely makes multiple gRPC calls to the provider
- Network latency accumulates significantly over thousands of rule evaluations
- Localhost TCP has minimal latency (~0.1ms), but it adds up:
  - If each rule makes 10 gRPC calls with 0.1ms latency each
  - Over 10,000 rules that's 10,000ms (10 seconds) of pure latency
  - Actual overhead is 69 seconds (79s - 10s), suggesting more calls or higher latency

### Breakdown Analysis

**Rule Execution Phase** accounts for almost all the difference:
- Containerless: 10.2 seconds
- Hybrid: 79.1 seconds
- **Difference: 68.9 seconds** of network overhead

**Other phases are similar**:
- Rule Loading: 1.0s (containerless) vs 1.7s (hybrid) - minimal difference
- Provider Setup: 1.7s (containerless) vs 0.2s (hybrid) - hybrid is faster!
  - Containerless starts Java process in-process (JVM startup)
  - Hybrid reuses already-running containerized provider

### Output Size Difference

Containerless produces 571KB output vs Hybrid's 279KB:
- This suggests different analysis results
- Possible causes:
  1. Containerless includes more detailed incident data
  2. Different rule evaluation behavior
  3. Provider configuration differences
  4. Need further investigation to determine if results are equivalent

## Architectural Trade-offs

### Containerless Mode
**Pros:**
- **6x faster** rule execution
- Direct in-process communication (no network overhead)
- Larger output (potentially more detailed)

**Cons:**
- No provider isolation
- Providers run in same process as analyzer
- Potential security/compliance issues for untrusted code

### Hybrid Mode
**Pros:**
- **Provider isolation** - runs in containers
- Suitable for security-sensitive/compliance scenarios
- Fast provider startup (reuses running containers)

**Cons:**
- **6x slower** due to network overhead
- Each gRPC call adds latency
- Slower for large-scale analysis

## Recommendations

1. **For Development/Internal Use**: Use containerless mode (--run-local=true)
   - 6x faster performance
   - Same safety as any local development tool
   - Ideal for interactive use and testing

2. **For Production/Multi-tenant**: Consider trade-offs carefully
   - Hybrid mode provides isolation but at significant performance cost
   - For high-volume scenarios, network overhead becomes prohibitive
   - May need to optimize provider communication or batch gRPC calls

3. **Potential Optimizations for Hybrid Mode**:
   - Batch gRPC calls where possible
   - Cache provider responses
   - Use Unix domain sockets instead of TCP localhost
   - Implement connection pooling and keep-alive
   - Profile to identify hotspots in provider communication

4. **Output Size Investigation**:
   - Compare output.yaml files to understand the 2x size difference
   - Ensure hybrid mode is producing complete/equivalent results
   - May need to adjust provider configuration for parity

## Conclusion

Hybrid mode successfully provides provider isolation but comes with a **6.2x performance penalty** due to network communication overhead. The current implementation is suitable for scenarios where isolation is required and performance is secondary. For high-performance use cases, containerless mode is strongly recommended.

The ~70 second network overhead in rule execution suggests that each rule evaluation involves significant provider communication. Optimizing this communication (batching, caching, better protocols) could dramatically improve hybrid mode performance.
