# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.24 AS builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.sum ./

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build
ARG TARGETARCH
RUN GOOS=linux GOARCH=$TARGETARCH go build -o evicted-pod-reaper cmd/manager/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
ARG TARGETARCH
FROM --platform=$TARGETARCH gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /workspace/evicted-pod-reaper .

USER 65532:65532

ENTRYPOINT ["/evicted-pod-reaper"]