group "default" {
    targets = ["reaper"]
}

variable "REGISTRY" {
    default = "public.ecr.aws"
}

variable "TAG" {
    default = "dev"
}

target "reaper" {
    context = "."
    dockerfile = "Dockerfile"
    platforms = ["linux/amd64", "linux/arm64"]
    tags = ["${REGISTRY}/kyos/evicted-pod-reaper:latest", "${REGISTRY}/kyos/evicted-pod-reaper:${TAG}"]
    push = false
}
