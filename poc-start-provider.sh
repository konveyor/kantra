#!/bin/bash
# POC: Start Java provider container with host network

set -e

echo "=== Starting Java Provider Container (POC) ==="
echo ""

# Configuration
PROVIDER_IMAGE="quay.io/konveyor/java-external-provider:latest"
PORT=9001
CONTAINER_NAME="poc-java-provider"

# Cleanup any existing container
echo "Cleaning up existing containers..."
podman rm -f ${CONTAINER_NAME} 2>/dev/null || true

# Create temporary directory for provider testing
TEST_INPUT=$(pwd)/pkg/testing/examples/ruleset/test-data/java
if [ ! -d "$TEST_INPUT" ]; then
    echo "❌ Test input directory not found: $TEST_INPUT"
    exit 1
fi

echo "✅ Test input: $TEST_INPUT"
echo ""

# Start provider container with host network
echo "Starting provider container..."
echo "  Image: $PROVIDER_IMAGE"
echo "  Network: host"
echo "  Port: $PORT"
echo "  Input: $TEST_INPUT"
echo ""

podman run -d \
    --name ${CONTAINER_NAME} \
    --network=host \
    -v "${TEST_INPUT}:/analyzer/input:ro" \
    ${PROVIDER_IMAGE} \
    --port=${PORT}

# Wait for provider to start
echo "Waiting for provider to start..."
sleep 3

# Check if container is running
if podman ps | grep -q ${CONTAINER_NAME}; then
    echo "✅ Provider container started successfully"
    echo ""
    echo "Container logs:"
    podman logs ${CONTAINER_NAME}
    echo ""
    echo "Provider should now be accessible at localhost:${PORT}"
    echo ""
    echo "Next steps:"
    echo "  1. Run: go run poc-test-provider.go"
    echo "  2. Stop provider: podman rm -f ${CONTAINER_NAME}"
else
    echo "❌ Provider container failed to start"
    podman logs ${CONTAINER_NAME}
    exit 1
fi
