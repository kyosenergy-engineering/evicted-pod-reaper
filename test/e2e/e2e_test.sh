#!/bin/bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Configuration
CLUSTER_NAME="evicted-pod-reaper-test"
NAMESPACE="test-evicted-pods"
IMAGE_NAME="evicted-pod-reaper:e2e-test"
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$SCRIPT_DIR/../.."

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Cleanup function
cleanup() {
    log_info "Cleaning up..."
    kubectl delete namespace $NAMESPACE --ignore-not-found=true || true
    kind delete cluster --name $CLUSTER_NAME || true
}

# Set up trap for cleanup on exit
trap cleanup EXIT

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v kind &> /dev/null; then
        log_error "kind is not installed. Please install kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
        exit 1
    fi

    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed. Please install kubectl."
        exit 1
    fi

    if ! command -v docker &> /dev/null; then
        log_error "docker is not installed. Please install docker."
        exit 1
    fi
}

# Create Kind cluster
create_cluster() {
    log_info "Creating Kind cluster..."
    kind create cluster --name $CLUSTER_NAME --config "$SCRIPT_DIR/kind-config.yaml"

    log_info "Waiting for cluster to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=60s
}

# Build and load image
build_and_load_image() {
    log_info "Building controller image..."
    cd "$PROJECT_ROOT"
    docker build -t $IMAGE_NAME .

    log_info "Loading image into Kind cluster..."
    kind load docker-image $IMAGE_NAME --name $CLUSTER_NAME
}

# Deploy controller
deploy_controller() {
    log_info "Creating namespace..."
    kubectl create namespace $NAMESPACE

    log_info "Deploying RBAC..."
    kubectl apply -f "$SCRIPT_DIR/manifests/rbac.yaml"

    log_info "Deploying controller with 5 second TTL..."
    kubectl apply -f "$SCRIPT_DIR/manifests/deployment.yaml"

    log_info "Waiting for controller to be ready..."
    kubectl -n $NAMESPACE wait --for=condition=available --timeout=60s deployment/evicted-pod-reaper

    # Give it a moment to start up completely
    sleep 5
}

# Test functions
create_evicted_pod() {
    local pod_name=$1
    local namespace=$2
    local preserve=${3:-false}

    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: $pod_name
  namespace: $namespace
  annotations:
    pod-reaper.kyos.com/preserve: "$preserve"
spec:
  containers:
  - name: test
    image: busybox
    command: ["sh", "-c", "exit 1"]
  restartPolicy: Never
EOF

    # Wait for pod to fail
    kubectl -n $namespace wait --for=condition=Failed pod/$pod_name --timeout=30s || true

    # Simulate eviction
    kubectl -n $namespace patch pod $pod_name --type='json' -p='[
        {"op": "replace", "path": "/status/phase", "value": "Failed"},
        {"op": "replace", "path": "/status/reason", "value": "Evicted"},
        {"op": "add", "path": "/status/message", "value": "The node was low on resource: memory."}
    ]' --subresource=status
}

wait_for_pod_deletion() {
    local pod_name=$1
    local namespace=$2
    local timeout=${3:-10}

    log_info "Waiting up to ${timeout}s for pod $pod_name to be deleted..."

    local count=0
    while kubectl -n $namespace get pod $pod_name &> /dev/null; do
        if [ $count -ge $timeout ]; then
            return 1
        fi
        sleep 1
        ((count++))
    done

    return 0
}

check_pod_exists() {
    local pod_name=$1
    local namespace=$2

    kubectl -n $namespace get pod $pod_name &> /dev/null
}

# Test scenarios
run_tests() {
    log_info "Running e2e tests..."

    # Test 1: Evicted pod should be deleted after TTL
    log_info "Test 1: Evicted pod should be deleted after TTL (5 seconds)"
    create_evicted_pod "test-pod-1" $NAMESPACE "false"

    if wait_for_pod_deletion "test-pod-1" $NAMESPACE 10; then
        log_info "✅ Test 1 PASSED: Evicted pod was deleted"
    else
        log_error "❌ Test 1 FAILED: Evicted pod was not deleted within timeout"
        exit 1
    fi

    # Test 2: Preserved pod should not be deleted
    log_info "Test 2: Preserved pod should not be deleted"
    create_evicted_pod "test-pod-2" $NAMESPACE "true"

    sleep 10  # Wait longer than TTL

    if check_pod_exists "test-pod-2" $NAMESPACE; then
        log_info "✅ Test 2 PASSED: Preserved pod was not deleted"
    else
        log_error "❌ Test 2 FAILED: Preserved pod was deleted"
        exit 1
    fi

    # Test 3: Non-evicted pod should not be deleted
    log_info "Test 3: Non-evicted pod should not be deleted"
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: test-pod-3
  namespace: $NAMESPACE
spec:
  containers:
  - name: test
    image: busybox
    command: ["sleep", "3600"]
EOF
    
    kubectl -n $NAMESPACE wait --for=condition=Ready pod/test-pod-3 --timeout=30s
    sleep 10  # Wait longer than TTL
    
    if check_pod_exists "test-pod-3" $NAMESPACE; then
        log_info "✅ Test 3 PASSED: Running pod was not deleted"
    else
        log_error "❌ Test 3 FAILED: Running pod was deleted"
        exit 1
    fi
    
    # Test 4: Pod in unwatched namespace should not be deleted
    log_info "Test 4: Pod in unwatched namespace should not be deleted"
    kubectl create namespace unwatched-ns || true
    create_evicted_pod "test-pod-4" "unwatched-ns" "false"

    sleep 10  # Wait longer than TTL

    if check_pod_exists "test-pod-4" "unwatched-ns"; then
        log_info "✅ Test 4 PASSED: Pod in unwatched namespace was not deleted"
    else
        log_error "❌ Test 4 FAILED: Pod in unwatched namespace was deleted"
        exit 1
    fi

    # Test 5: Check metrics
    log_info "Test 5: Checking Prometheus metrics"

    # Port-forward to access metrics
    kubectl -n $NAMESPACE port-forward deployment/evicted-pod-reaper 8080:8080 &
    PF_PID=$!
    sleep 2

    # Fetch metrics
    METRICS=$(curl -s http://localhost:8080/metrics || echo "")
    kill $PF_PID 2>/dev/null || true
    
    if echo "$METRICS" | grep -q "evicted_pods_deleted_total"; then
        log_info "✅ Test 5 PASSED: Metrics endpoint is working"
        echo "$METRICS" | grep "evicted_pods_"
    else
        log_error "❌ Test 5 FAILED: Metrics endpoint not working properly"
        exit 1
    fi
}

# Main execution
main() {
    log_info "Starting e2e tests for evicted-pod-reaper..."

    check_prerequisites
    create_cluster
    build_and_load_image
    deploy_controller
    run_tests

    log_info "🎉 All e2e tests passed!"
}

# Run main function
main