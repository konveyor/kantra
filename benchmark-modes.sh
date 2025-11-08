#!/bin/bash
# Benchmark script to compare containerless vs hybrid mode performance
# This validates the performance claims in HYBRID_MODE.md

set -e

# Configuration
TEST_APP="pkg/testing/examples/ruleset/test-data/java"
ITERATIONS=3
KANTRA_BIN="./kantra"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Kantra Mode Benchmark${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo "Test Application: $TEST_APP"
echo "Iterations per mode: $ITERATIONS"
echo "Platform: $(uname -s) $(uname -m)"
echo ""

# Check if kantra binary exists
if [ ! -f "$KANTRA_BIN" ]; then
    echo -e "${RED}Error: Kantra binary not found at $KANTRA_BIN${NC}"
    echo "Build it first: go build -o kantra"
    exit 1
fi

# Check if test app exists
if [ ! -d "$TEST_APP" ]; then
    echo -e "${RED}Error: Test application not found at $TEST_APP${NC}"
    exit 1
fi

# Function to clean up output directories
cleanup_outputs() {
    echo -e "${YELLOW}Cleaning up previous outputs...${NC}"
    rm -rf /tmp/benchmark-containerless-*
    rm -rf /tmp/benchmark-hybrid-*
    # Stop any running provider containers
    podman stop $(podman ps -a | grep provider | awk '{print $1}') 2>/dev/null || true
    podman rm $(podman ps -a | grep provider | awk '{print $1}') 2>/dev/null || true
}

# Function to run analysis and measure time
run_analysis() {
    local mode=$1
    local iteration=$2
    local output_dir=$3
    local run_local=$4

    # Capture start time (nanoseconds for precision)
    start=$(date +%s%N)

    # Run analysis (suppress output, capture only timing)
    if [ "$run_local" = "true" ]; then
        $KANTRA_BIN analyze \
            --input "$TEST_APP" \
            --output "$output_dir" \
            --run-local=true \
            --mode source-only \
            --overwrite \
            > /dev/null 2>&1
    else
        $KANTRA_BIN analyze \
            --input "$TEST_APP" \
            --output "$output_dir" \
            --run-local=false \
            --mode source-only \
            --overwrite \
            > /dev/null 2>&1
    fi

    # Capture end time
    end=$(date +%s%N)

    # Calculate duration in milliseconds
    duration=$(( ($end - $start) / 1000000 ))

    # Print to stderr so it doesn't interfere with return value
    echo -e "${YELLOW}  Run $iteration/$ITERATIONS: ${GREEN}${duration}ms${NC}" >&2

    # Return just the number
    echo $duration
}

# Cleanup before starting
cleanup_outputs

# Array to store timings
declare -a containerless_times
declare -a hybrid_times

# Benchmark Containerless Mode
echo -e "\n${BLUE}Benchmarking Containerless Mode (--run-local=true)${NC}"
echo -e "${BLUE}================================================${NC}"
for i in $(seq 1 $ITERATIONS); do
    output_dir="/tmp/benchmark-containerless-$i"
    time_ms=$(run_analysis "containerless" $i "$output_dir" "true")
    containerless_times+=($time_ms)
done

# Wait a bit between modes
echo -e "\n${YELLOW}Waiting 5 seconds before switching modes...${NC}"
sleep 5

# Benchmark Hybrid Mode
echo -e "\n${BLUE}Benchmarking Hybrid Mode (--run-local=false)${NC}"
echo -e "${BLUE}=============================================${NC}"
for i in $(seq 1 $ITERATIONS); do
    output_dir="/tmp/benchmark-hybrid-$i"
    time_ms=$(run_analysis "hybrid" $i "$output_dir" "false")
    hybrid_times+=($time_ms)
done

# Calculate averages
containerless_avg=0
for time in "${containerless_times[@]}"; do
    containerless_avg=$((containerless_avg + time))
done
containerless_avg=$((containerless_avg / ITERATIONS))

hybrid_avg=0
for time in "${hybrid_times[@]}"; do
    hybrid_avg=$((hybrid_avg + time))
done
hybrid_avg=$((hybrid_avg / ITERATIONS))

# Calculate speedup
if [ $hybrid_avg -gt 0 ]; then
    speedup=$(echo "scale=2; $containerless_avg / $hybrid_avg" | bc)
else
    speedup="N/A"
fi

# Display Results
echo -e "\n${BLUE}========================================${NC}"
echo -e "${BLUE}Benchmark Results${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "${YELLOW}Containerless Mode (--run-local=true):${NC}"
echo "  Individual runs: ${containerless_times[@]} ms"
echo "  Average: ${containerless_avg} ms"
echo ""
echo -e "${YELLOW}Hybrid Mode (--run-local=false):${NC}"
echo "  Individual runs: ${hybrid_times[@]} ms"
echo "  Average: ${hybrid_avg} ms"
echo ""
echo -e "${GREEN}========================================${NC}"
if (( $(echo "$speedup > 1" | bc -l) )); then
    echo -e "${GREEN}Hybrid is ${speedup}x FASTER than Containerless${NC}"
elif (( $(echo "$speedup < 1" | bc -l) )); then
    inverse=$(echo "scale=2; 1 / $speedup" | bc)
    echo -e "${RED}Hybrid is ${inverse}x SLOWER than Containerless${NC}"
else
    echo -e "${YELLOW}Both modes have similar performance${NC}"
fi
echo -e "${GREEN}========================================${NC}"
echo ""

# Detailed breakdown
echo -e "${BLUE}Detailed Breakdown:${NC}"
printf "%-20s %-15s %-15s\n" "Run" "Containerless" "Hybrid"
printf "%-20s %-15s %-15s\n" "---" "-------------" "------"
for i in $(seq 0 $((ITERATIONS-1))); do
    printf "%-20s %-15s %-15s\n" "Run $((i+1))" "${containerless_times[$i]} ms" "${hybrid_times[$i]} ms"
done
echo ""

# Cleanup
echo -e "${YELLOW}Cleaning up benchmark outputs...${NC}"
cleanup_outputs
echo -e "${GREEN}Benchmark complete!${NC}"
