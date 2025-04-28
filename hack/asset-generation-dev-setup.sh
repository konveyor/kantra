#!/bin/sh
# This script sets up a local development environment for Korifi using KinD.
# It checks for prerequisites, creates a KinD cluster, and deploys Korifi.


set -e

RUNTIME=""
CLEANUP=false
VERSION="1.0.0"

print_help() {
  cat <<EOF
Usage: $0 [--docker | --podman] [--cleanup] [--version] [--help]

Flags:
  --docker        Use Docker as the container runtime
  --podman        Use Podman as the container runtime
  --cleanup       Delete kind cluster and Korifi installation
  --version       Show the version of the script
  --help          Show this help message

Note:
  If no runtime is specified, Podman will be used by default.
EOF
}

parse_args() {
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      --docker)
        RUNTIME="docker"
        ;;
      --podman)
        RUNTIME="podman"
        ;;
      --cleanup)
        CLEANUP=true
        ;;
      --version)
        echo "Version: ${VERSION}"
        exit 0
        ;;
      --help)
        print_help
        exit 0
        ;;
      *)
        echo "❌ Unknown option: $1"
        print_help
        exit 1
        ;;
    esac
    shift
  done

  if [[ -z "$RUNTIME" ]]; then
    echo "ℹ️ No runtime specified. Defaulting to Podman."
    RUNTIME="podman"
  fi
}

setup_runtime_env() {
  if [[ "$RUNTIME" == "docker" ]]; then
    if ! command -v docker &>/dev/null; then
      echo "❌ Docker isn't istalled."
      echo "Official instructions available in: https://docs.docker.com/get-docker/"
      echo "Be sure to have rootless Docker installed (https://docs.docker.com/engine/security/rootless/)."
      exit 1
    fi
    export DOCKER_HOST=unix://$XDG_RUNTIME_DIR/docker.sock
    echo "✅ Using Docker 🐳"
  elif [[ "$RUNTIME" == "podman" ]]; then
    if ! command -v podman &>/dev/null; then
      echo "❌ Podman is not installed."
      echo "Official instructions available in: https://podman.io/getting-started/installation"
      exit 1
    fi
    echo "✅ Using Podman 🦭"
  fi
}

run_kind() {
  if [[ "$RUNTIME" == "podman" ]]; then
    KIND_EXPERIMENTAL_PROVIDER=podman kind "$@"
  else
    kind "$@"
  fi
}
check_prerequisite(){
  if ! command -v go &> /dev/null; then
      echo "❌ Go could not be found. Please install it."
      echo "Official instructions available at https://go.dev/doc/install"
      exit 1
  fi

  # Check Go version
  local min_go_version="1.22.9"
  local current_go_version="$(go env GOVERSION)"
  current_go_version="${current_go_version#go}"

  if [ "$(printf '%s\n' "${min_go_version}" "${current_go_version}" | sort -V | head -n1)" != "1.22.9" ]; then
      echo "❌ Incorrect Go version. Please install Go version 1.22.9 or higher."
      echo "Official instructions available at https://go.dev/doc/install"
      exit 1
  fi

  # Check if kubectl is available
  if ! command -v kubectl &> /dev/null; then
      echo "❌ kubectl could not be found. Please install it"
      echo "Official instructions available at https://kubernetes.io/docs/tasks/tools/"
      exit 1
  fi

  # Check if cf CLI v8 is available
  if cf version 2>/dev/null | grep -qE '^cf version 8\.'; then
      echo "❌ cf CLI v8 could not be found. Please install it"
      echo "Official instructions available at https://docs.cloudfoundry.org/cf-cli/install-go-cli.html"
      exit 1
  fi

  # Check if kind is installed
  if ! command -v kind &> /dev/null; then
      echo "kind could not be found. Trying to installing it now..."
      go install sigs.k8s.io/kind@v0.27.0
      export PATH=$PATH:$(go env GOPATH)/bin/
      # Recheck installation
      if command -v kind &> /dev/null; then
          echo "✅ kind installed successfully!"
      else
          echo "❌ kind installation failed or not in PATH."
          exit 1
      fi
      
  else
      echo "kind is already installed."
  fi
  echo "All prerequisites are met."
}

cleanup() {
  echo "🧹 Cleaning up..."
  echo "⛔ Deleting kind cluster 'korifi' (if it exists)..."
  run_kind delete cluster --name korifi
  echo "✅ Cleanup complete."
  exit 0
}

# wait_for_pods_to_exist waits for pods in a given namespace with a specific name prefix
# to exist until the max wait time is reached.
# Arguments:
#   1. Namespace
#   2. Pod name prefix
#   3. Max wait time in seconds
# Returns:
#   0 if pods exist
#   1 if pods do not exist within the max wait time
# Usage:
#   wait_for_pods_to_exist <namespace> <pod_name_prefix> <max_wait_secs>
# Example:
#   wait_for_pods_to_exist "my-namespace" "my-pod-prefix" 300
wait_for_pods_to_exist() {
  local ns=$1
  local pod_name_prefix=$2
  local max_wait_secs=$3
  local interval_secs=2
  local start_time
  start_time=$(date +%s)
  while true; do

    current_time=$(date +%s)
    if (( (current_time - start_time) > max_wait_secs )); then
      echo "Waited for pods in namespace \"$ns\" with name prefix \"$pod_name_prefix\" to exist for $max_wait_secs seconds without luck. Returning with error."
      return 1
    fi

    if kubectl -n "$ns" describe pod "$pod_name_prefix" --request-timeout "5s"  &> /dev/null; then
      # echo "Pods in namespace \"$ns\" with name prefix \"$pod_name_prefix\" exist."
      break
    else
      sleep $interval_secs
    fi
  done
}

create_cluster(){

  CLUSTER_NAME="korifi"
  # Check if the cluster exists
  if run_kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
      echo "✅ Kind cluster '${CLUSTER_NAME}' already exists. Skipping creation."
  else
      echo "🚀 Creating korifi kind cluster..."

      cat <<EOF | run_kind create cluster --name ${CLUSTER_NAME} --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localregistry-docker-registry.default.svc.cluster.local:30050"]
        endpoint = ["http://127.0.0.1:30050"]
    [plugins."io.containerd.grpc.v1.cri".registry.configs]
      [plugins."io.containerd.grpc.v1.cri".registry.configs."127.0.0.1:30050".tls]
        insecure_skip_verify = true
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 32080
    hostPort: 80
    protocol: TCP
  - containerPort: 32443
    hostPort: 443
    protocol: TCP
  - containerPort: 30050
    hostPort: 30050
    protocol: TCP
EOF
  fi

  # ✅ Check if the cluster creation was successful
  if [ $? -eq 0 ]; then
    echo "✅ Kind cluster 'korifi' created successfully!"
  else
    echo "❌ Failed to create kind cluster 'korifi'"
    exit 1
  fi
}

deploy_korifi() {
  echo "🚀 Deploying Korifi..."
  kubectl apply -f https://github.com/cloudfoundry/korifi/releases/latest/download/install-korifi-kind.yaml
  
  echo "⏳ Waiting for Korifi installer to be ready..."
  wait_for_pods_to_exist "korifi-installer" "install-korifi" 180
  kubectl wait pod -n korifi-installer -l job-name=install-korifi --for=condition=Ready --timeout=180s
  # Waiting for Korifi to be ready
  echo "⏳ Waiting for Korifi to be ready..."
  kubectl -n korifi-installer logs --follow job/install-korifi

  # ✅ Check if the cluster creation was successful
  if [ $? -eq 0 ]; then
    echo "✅ Korifi is ready!"
  else
    echo "❌ Failed to deploy korifi"
    exit 1
  fi
}

main() {
  parse_args "$@"
  setup_runtime_env

  if [[ "$CLEANUP" == true ]]; then
    cleanup
    exit 0
  fi

  echo "🚀 Proceeding with Korifi setup using $RUNTIME..."

  check_prerequisite
  create_cluster
  deploy_korifi

  echo "Now you can deploy your CloudFoundry apps to the Korifi cluster."
  echo "To access Korifi use the instructions below:"
  echo 'https://github.com/cloudfoundry/korifi/blob/main/INSTALL.kind.md#test-korifi'
  echo "To access via cURL to the API endpoint, use the following command:"
  echo ">> curl -k -H \"X-Username: kubernetes-admin\" -H \"Authorization: ClientCert \$(yq '.users[] | select (.name == \\\"kind-korifi\\\") | .user.client-certificate-data' \"\$HOME/.kube/config\")\$(yq '.users[] | select (.name == \\\"kind-korifi\\\") | .user.client-key-data' \"\$HOME/.kube/config\")\" https://localhost/v3/apps | jq"
}

main "$@"
