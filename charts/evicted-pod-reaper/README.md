# evicted-pod-reaper

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.0](https://img.shields.io/badge/AppVersion-1.0.0-informational?style=flat-square)

A Kubernetes operator that automatically deletes evicted pods after a configurable TTL

## TL;DR

```bash
helm install evicted-pod-reaper oci://ghcr.io/kyosenergy-engineering/evicted-pod-reaper --version 1.0.0
```

## Introduction

This chart bootstraps an evicted-pod-reaper deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

The evicted-pod-reaper is a Kubernetes controller that watches for evicted pods and automatically deletes them after a configurable time-to-live (TTL). This helps keep your cluster clean from evicted pods that can accumulate over time.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+

## Installing the Chart

To install the chart with the release name `evicted-pod-reaper`:

```bash
helm install evicted-pod-reaper ./charts/evicted-pod-reaper
```

The command deploys evicted-pod-reaper on the Kubernetes cluster with default configuration. The [Values](#values) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `evicted-pod-reaper` deployment:

```bash
helm delete evicted-pod-reaper
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following table lists the configurable parameters of the evicted-pod-reaper chart and their default values.

### Controller Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas to deploy | `1` |
| `controller.leaderElection` | Enable leader election for controller (recommended for HA) | `false` |
| `controller.healthProbeBindAddress` | Health probe bind address | `:8081` |
| `controller.metricsBindAddress` | Metrics bind address | `:8080` |

### Reaper Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `reaper.watchAllNamespaces` | Whether to watch all namespaces. If false, uses watchNamespaces | `false` |
| `reaper.watchNamespaces` | List of namespaces to watch (ignored if watchAllNamespaces is true) | `["default"]` |
| `reaper.ttlToDelete` | Time in seconds to wait before deleting an evicted pod | `300` |
| `reaper.env` | Additional environment variables | `[]` |

### Image Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Container image repository | `public.ecr.aws/kyos/evicted-pod-reaper` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Overrides the image tag (default is chart appVersion) | `""` |
| `imagePullSecrets` | Image pull secrets for private registries | `[]` |

### Security Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podSecurityContext` | Pod security context | See values.yaml |
| `securityContext` | Container security context | See values.yaml |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `serviceAccount.name` | Service account name | `""` |
| `rbac.create` | Create RBAC resources | `true` |
| `rbac.additionalRules` | Additional RBAC rules | `[]` |

### Monitoring Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `metrics.enabled` | Enable metrics | `true` |
| `metrics.service.annotations` | Metrics service annotations | `{}` |
| `metrics.service.labels` | Metrics service labels | `{}` |
| `metrics.podMonitor.enabled` | Enable PodMonitor creation | `false` |
| `metrics.podMonitor.namespace` | Namespace for PodMonitor | `""` |
| `metrics.podMonitor.interval` | Scrape interval | `30s` |
| `metrics.podMonitor.scrapeTimeout` | Scrape timeout | `10s` |

### Resource Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `64Mi` |

### Advanced Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `autoscaling.enabled` | Enable autoscaling | `false` |
| `autoscaling.minReplicas` | Minimum replicas | `1` |
| `autoscaling.maxReplicas` | Maximum replicas | `3` |
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Tolerations | `[]` |
| `affinity` | Affinity rules | `{}` |
| `podDisruptionBudget.enabled` | Enable PodDisruptionBudget | `false` |
| `networkPolicy.enabled` | Enable NetworkPolicy | `false` |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example:

```bash
helm install evicted-pod-reaper ./charts/evicted-pod-reaper \
  --set reaper.ttlToDelete=600 \
  --set reaper.watchAllNamespaces=true
```

Alternatively, a YAML file that specifies the values for the parameters can be provided while installing the chart. For example:

```bash
helm install evicted-pod-reaper ./charts/evicted-pod-reaper -f values.yaml
```

## Namespace vs Cluster-wide Operation

The evicted-pod-reaper can operate in two modes:

### Namespace-scoped (Default)
- Set `reaper.watchAllNamespaces: false`
- Specify namespaces in `reaper.watchNamespaces`
- Creates Role and RoleBinding in the release namespace
- More secure, follows principle of least privilege

### Cluster-wide
- Set `reaper.watchAllNamespaces: true`
- Creates ClusterRole and ClusterRoleBinding
- Can monitor and clean evicted pods across all namespaces
- Requires cluster-admin to install

## Monitoring

The evicted-pod-reaper exposes Prometheus metrics on port 8080 at `/metrics`. The following metrics are available:

- `evicted_pod_reaper_pods_deleted_total`: Total number of evicted pods deleted (labeled by namespace)
- `evicted_pod_reaper_pods_skipped_total`: Total number of evicted pods skipped due to preserve annotation (labeled by namespace)

To enable automatic metrics collection with Prometheus Operator:

```bash
helm install evicted-pod-reaper ./charts/evicted-pod-reaper \
  --set metrics.podMonitor.enabled=true
```

## High Availability

For production deployments, consider enabling:

1. **Leader Election**: Prevents multiple instances from processing the same pods
   ```yaml
   controller:
     leaderElection: true
   ```

2. **Multiple Replicas**: Ensures availability during node failures
   ```yaml
   replicaCount: 2
   ```

3. **Pod Disruption Budget**: Maintains availability during cluster maintenance
   ```yaml
   podDisruptionBudget:
     enabled: true
     minAvailable: 1
   ```

## Preserving Evicted Pods

To prevent specific evicted pods from being deleted, add the following annotation:

```yaml
metadata:
  annotations:
    evicted-pod-reaper.kyos.io/preserve: "true"
```

## Troubleshooting

### Controller not deleting evicted pods
1. Check controller logs: `kubectl logs -l app.kubernetes.io/name=evicted-pod-reaper`
2. Verify RBAC permissions are correct
3. Ensure the controller is watching the correct namespaces

### Metrics not being scraped
1. Verify PodMonitor is created: `kubectl get podmonitor`
2. Check Prometheus operator is installed and configured
3. Ensure network policies allow traffic from Prometheus

## Contributing

Please see the [contributing guide](https://github.com/kyos/evicted-pod-reaper/blob/master/CONTRIBUTING.md) if you are interested in contributing.
