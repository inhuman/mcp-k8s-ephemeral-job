# syntax=docker/dockerfile:1

# Build stage — uses the committed vendor tree for reproducible builds.
# Pinned by multi-arch manifest-list digest (constitution X). Refresh intentionally via:
#   docker buildx imagetools inspect golang:1.26-bookworm
FROM golang:1.26-bookworm@sha256:5d2b868674b57c9e48cdd39e891acce4196b6926ca6d11e9c270a8f85106203d AS build
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -mod=vendor -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/mcp-k8s-ephemeral-job ./cmd/mcp-k8s-ephemeral-job

# Runtime stage — distroless static, non-root. No shell, no package manager.
# NOTE: pin by @sha256 digest at first release (T036); refresh via:
#   docker buildx imagetools inspect gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=build /out/mcp-k8s-ephemeral-job /usr/local/bin/mcp-k8s-ephemeral-job
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/mcp-k8s-ephemeral-job"]
