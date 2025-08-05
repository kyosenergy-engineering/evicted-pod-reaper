# E2E Tests for Evicted Pod Reaper

This directory contains end-to-end tests for the evicted-pod-reaper controller.

## Prerequisites

- Docker
- Kind (Kubernetes in Docker)
- kubectl
- Go 1.22+ (for Go-based tests)

## Running Tests

### Using the Shell Script (Recommended)

The shell script handles everything automatically:

```bash
# From project root
make e2e-test

# Or run directly
./test/e2e/e2e_test.sh
```

This will:
1. Create a Kind cluster
2. Build and load the controller image
3. Deploy the controller with a 5-second TTL
4. Run all test scenarios
5. Clean up resources

### Using Go Tests

If you already have a cluster with the controller deployed:

```bash
# Run e2e tests
go test -tags=e2e ./test/e2e/...

# With specific kubeconfig
KUBECONFIG=/path/to/kubeconfig go test -tags=e2e ./test/e2e/...
```

## Test Scenarios

1. **Evicted Pod Deletion**: Verifies that evicted pods are deleted after TTL (5 seconds)
2. **Preserved Pod**: Verifies that pods with `pod-reaper.kyos.com/preserve: "true"` are not deleted
3. **Running Pod**: Verifies that running pods are not affected
4. **Unwatched Namespace**: Verifies that pods in unwatched namespaces are not deleted
5. **Metrics Endpoint**: Verifies that Prometheus metrics are exposed correctly

## Configuration

The e2e tests use a 5-second TTL instead of the default 300 seconds to make tests run faster.

Environment variables used in tests:
- `REAPER_WATCH_NAMESPACES`: "test-evicted-pods"
- `REAPER_TTL_TO_DELETE`: "5"
- `REAPER_WATCH_ALL_NAMESPACES`: "false"

## Troubleshooting

If tests fail:

1. Check if Kind cluster is running:
   ```bash
   kind get clusters
   ```

2. Check controller logs:
   ```bash
   kubectl -n test-evicted-pods logs -l app=evicted-pod-reaper
   ```

3. Check pod status:
   ```bash
   kubectl -n test-evicted-pods get pods
   ```

4. Clean up and retry:
   ```bash
   kind delete cluster --name evicted-pod-reaper-test
   ./test/e2e/e2e_test.sh
   ```