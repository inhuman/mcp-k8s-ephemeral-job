# syntax=docker/dockerfile:1

# Build stage — uses the committed vendor tree for reproducible builds.
# Pinned by multi-arch manifest-list digest (constitution X). Refresh intentionally via:
#   docker buildx imagetools inspect golang:1.26-bookworm
FROM golang:1.26-bookworm@sha256:18aedc16aa19b3fd7ded7245fc14b109e054d65d22ed53c355c899582bbb2113 AS build
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -mod=vendor -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/mcp-k8s-ephemeral-job ./cmd/mcp-k8s-ephemeral-job

# Runtime stage — distroless static, non-root. No shell, no package manager.
# Pinned by multi-arch manifest-list digest. Update the digest intentionally when
# bumping the base; refresh via:
#   docker buildx imagetools inspect gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12:nonroot@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b AS runtime
# Ownership proof for the MCP Registry: the label value MUST equal the `name`
# field in server.json. https://registry.modelcontextprotocol.io
LABEL io.modelcontextprotocol.server.name="io.github.inhuman/mcp-k8s-ephemeral-job" \
      org.opencontainers.image.source="https://github.com/inhuman/mcp-k8s-ephemeral-job"
COPY --from=build /out/mcp-k8s-ephemeral-job /usr/local/bin/mcp-k8s-ephemeral-job
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/mcp-k8s-ephemeral-job"]
