# ☠️ evicted-pod-reaper

A Kubernetes operator that automatically deletes `Evicted` pods after a TTL, unless explicitly preserved via annotation.

Built using [Kubebuilder](https://book.kubebuilder.io/) and `controller-runtime`. Lightweight, fast, and extensible.

## 🎯 Features

- 🧹 Deletes pods with:
  - `status.phase == Failed`
  - `status.reason == "Evicted"`
- 🔒 Skips pods with annotation: `pod-reaper.kyos.com/preserve: "true"`
- 🌐 Watches only specified namespaces via ENV
- 🔰 Only deletes pods after the specified TTL has passed
- 📊 Prometheus metrics:
  - `evicted_pods_deleted_total`
  - `evicted_pods_skipped_total`
- ⚙️ No CRDs, simple RBAC

## 🛠️ Environment Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `REAPER_WATCH_ALL_NAMESPACES` | `true/false` | `false` | If true, watches all namespaces |
| `REAPER_WATCH_NAMESPACES` | `csv` | `default` | Comma-separated list of namespaces (e.g. `kube-system,monitoring`) |
| `REAPER_TTL_TO_DELETE` | `int` | 300 | Number of seconds to wait before deleting an evicted pod (TTL) |

> Effectively making the default behavior, when `REAPER_WATCH_ALL_NAMESPACES` and `REAPER_WATCH_NAMESPACES` are  not set, to only delete Pods in the `default` namespace.

## 🧪 Reaper Logic

```go
if pod.Status.Phase == "Failed" && pod.Status.Reason == "Evicted" {
  if pod.Annotations["pod-reaper.kyos.com/preserve"] == "true" {
    // skip
  } else {
    client.Delete(pod)
    metrics.IncDeleted(namespace)
  }
}
````

## 📦 Metrics

Exposed on `/metrics` (Prometheus format):

- `evicted_pods_deleted_total{namespace="..."}`
- `evicted_pods_skipped_total{namespace="..."}`

## 🔐 RBAC

```yaml
apiGroups: [""]
resources: ["pods"]
verbs: ["get", "list", "watch", "delete"]
```

Use a `ClusterRole` if watching all namespaces. Otherwise, apply a `Role` scoped to each watched namespace.
By default, the Helm chart creates a `ClusterRole` and `ClusterRoleBinding`.

## 🐳 Dockerfile

```dockerfile
FROM golang:1.24 AS builder
WORKDIR /workspace
COPY . .
RUN make manager

FROM gcr.io/distroless/static
COPY --from=builder /workspace/bin/manager /
ENTRYPOINT ["/manager"]
```

## 🚀 Example Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: evicted-pod-reaper
spec:
  replicas: 1
  selector:
    matchLabels:
      app: evicted-pod-reaper
  template:
    metadata:
      labels:
        app: evicted-pod-reaper
    spec:
      serviceAccountName: pod-reaper
      containers:
        - name: reaper
          image: public.ecr.aws/kyos/evicted-pod-reaper:latest
          env:
            - name: REAPER_WATCH_NAMESPACES
              value: "kube-system,monitoring"
```

### ☸️ Helm

A helm chart is available for easy deployment, using best practices.

## 👨‍🔧 Local Development

```bash
# Build and run locally
make install
make run
```

## 🧪 Test Cases

* ✅ Evicted pod → deleted
* 🚫 Not evicted → ignored
* ✋ Annotated pod with value `true` → preserved
* ✋ Annotated pod with value `false` → deleted
* 📦 Wrong namespace → ignored

## 🙋 FAQ

**Does this touch running pods?**
No — it only touches `Failed` pods with `"Evicted"` reason.

## 📄 License

MIT

