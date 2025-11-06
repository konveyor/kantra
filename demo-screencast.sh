#!/usr/bin/env bash

# Kantra Analysis Mode Comparison Demo
# This script demonstrates different analysis modes and their performance
#
# Prerequisites:
#   1. Build kantra: go build -o ./bin/kantra
#   2. Have Podman/Docker installed
#   3. Maven installed (for building test WAR)

set -e  # Exit on error

# Check if kantra binary exists
if [ ! -f "./bin/kantra" ]; then
    echo -e "\033[1;31mError: ./bin/kantra not found${NC}"
    echo -e "\033[1;33mPlease build it first: go build -o ./bin/kantra${NC}"
    exit 1
fi

# Colors for output
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test inputs
JAVA_SOURCE="pkg/testing/examples/ruleset/test-data/java"
JAVA_BINARY="pkg/testing/examples/ruleset/test-data/java/target/customers-tomcat-0.0.1-SNAPSHOT.war"

# Build the WAR file if it doesn't exist
if [ ! -f "$JAVA_BINARY" ]; then
    echo -e "${YELLOW}Building test WAR file...${NC}"
    (cd pkg/testing/examples/ruleset/test-data/java && mvn clean package -DskipTests -q) || {
        echo -e "${YELLOW}Warning: Could not build WAR file. Skipping binary tests.${NC}"
        SKIP_BINARY=true
    }
    echo ""
fi

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Kantra Analysis Mode Comparison${NC}"
echo -e "${BLUE}========================================${NC}\n"

echo -e "${YELLOW}Testing 2 modes × 2 input types = 4 scenarios:${NC}"
echo -e "  • Containerless Mode (--run-local=true)"
echo -e "  • Hybrid Mode (--run-local=false, default)"
echo -e "  × Source Code Analysis"
echo -e "  × Binary Analysis (WAR)\n"

echo -e "${YELLOW}Note: Using local output directories to avoid macOS /tmp limitations${NC}\n"

# Create output directory
OUTPUT_DIR="./demo-output"
mkdir -p "$OUTPUT_DIR"

# ========================================
# CONTAINERLESS MODE
# ========================================

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}CONTAINERLESS MODE${NC}"
echo -e "${BLUE}========================================${NC}\n"

# Demo 1: Containerless Source Analysis
echo -e "${GREEN}[1/4] Containerless - Source Code Analysis${NC}"
echo -e "Mode: Containerless (everything runs on host)"
echo -e "Input: Java source code"
echo -e "Command: kantra analyze --input $JAVA_SOURCE --output $OUTPUT_DIR/containerless-source --overwrite --target quarkus --run-local=true\n"

time ./bin/kantra analyze \
  --input "$JAVA_SOURCE" \
  --output "$OUTPUT_DIR/containerless-source" \
  --overwrite \
  --target quarkus \
  --run-local=true

echo -e "\n${BLUE}========================================${NC}\n"
sleep 2

# Demo 2: Containerless Binary Analysis
if [ "$SKIP_BINARY" != "true" ]; then
    echo -e "${GREEN}[2/4] Containerless - Binary Analysis${NC}"
    echo -e "Mode: Containerless (everything runs on host)"
    echo -e "Input: WAR file"
    echo -e "Command: kantra analyze --input $JAVA_BINARY --output $OUTPUT_DIR/containerless-binary --overwrite --target quarkus --run-local=true\n"

    time ./bin/kantra analyze \
      --input "$JAVA_BINARY" \
      --output "$OUTPUT_DIR/containerless-binary" \
      --overwrite \
      --target quarkus \
      --run-local=true

    echo -e "\n${BLUE}========================================${NC}\n"
    sleep 2
else
    echo -e "${YELLOW}[2/4] Containerless - Binary Analysis (SKIPPED - no WAR file)${NC}\n"
    echo -e "${BLUE}========================================${NC}\n"
    sleep 1
fi

# ========================================
# HYBRID MODE (DEFAULT)
# ========================================

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}HYBRID MODE (DEFAULT)${NC}"
echo -e "${BLUE}========================================${NC}\n"

# Demo 3: Hybrid Source Analysis
echo -e "${GREEN}[3/4] Hybrid - Source Code Analysis${NC}"
echo -e "Mode: Hybrid (analyzer on host, providers in containers)"
echo -e "Input: Java source code"
echo -e "Command: kantra analyze --input $JAVA_SOURCE --output $OUTPUT_DIR/hybrid-source --overwrite --target quarkus --run-local=false\n"

time ./bin/kantra analyze \
  --input "$JAVA_SOURCE" \
  --output "$OUTPUT_DIR/hybrid-source" \
  --overwrite \
  --target quarkus \
  --run-local=false

echo -e "\n${BLUE}========================================${NC}\n"
sleep 2

# Demo 4: Hybrid Binary Analysis
if [ "$SKIP_BINARY" != "true" ]; then
    echo -e "${GREEN}[4/4] Hybrid - Binary Analysis${NC}"
    echo -e "Mode: Hybrid (analyzer on host, providers in containers)"
    echo -e "Input: WAR file"
    echo -e "Command: kantra analyze --input $JAVA_BINARY --output $OUTPUT_DIR/hybrid-binary --overwrite --target quarkus --run-local=false\n"

    time ./bin/kantra analyze \
      --input "$JAVA_BINARY" \
      --output "$OUTPUT_DIR/hybrid-binary" \
      --overwrite \
      --target quarkus \
      --run-local=false
else
    echo -e "${YELLOW}[4/4] Hybrid - Binary Analysis (SKIPPED - no WAR file)${NC}\n"
fi

echo -e "\n${BLUE}========================================${NC}"
echo -e "${GREEN}Demo Complete!${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "\n${YELLOW}Output directories:${NC}"
echo -e "  ✅ Containerless Source: $OUTPUT_DIR/containerless-source"
if [ "$SKIP_BINARY" != "true" ]; then
    echo -e "  ✅ Containerless Binary: $OUTPUT_DIR/containerless-binary"
fi
echo -e "  ✅ Hybrid Source:        $OUTPUT_DIR/hybrid-source"
if [ "$SKIP_BINARY" != "true" ]; then
    echo -e "  ✅ Hybrid Binary:        $OUTPUT_DIR/hybrid-binary"
fi
echo -e "\n${YELLOW}Performance comparison: Check the 'time' output above${NC}"
echo -e "${YELLOW}View reports: open $OUTPUT_DIR/*/static-report/index.html${NC}"
echo -e "\n${YELLOW}Summary:${NC}"
echo -e "  • Containerless mode: Fast, requires local tooling"
echo -e "  • Hybrid mode: Fast + provider isolation (recommended)"
echo -e "  • Both modes support source code and binary (WAR/JAR/EAR) analysis"
