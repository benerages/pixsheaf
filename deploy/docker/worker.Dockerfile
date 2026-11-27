# syntax=docker/dockerfile:1
# pixsheaf-worker: isolated image worker (ADR 0004, 0006).
# Phase 2: static Go binary only. Phase 3 adds the libvips source build
# (pinned tarballs, cgo) described in ADR 0004.
# Build from the repository root:
#   docker build -f deploy/docker/worker.Dockerfile -t pixsheaf-worker .

ARG GO_IMAGE=golang:1.27.4-trixie@sha256:430985542100b871e108ccc00e8dcc5eef08ca14ee5825e98ebf563d8ffec4ee
ARG RUNTIME_IMAGE=debian:trixie-slim

FROM ${GO_IMAGE} AS build
ENV GOTOOLCHAIN=local CGO_ENABLED=0
WORKDIR /src
COPY go.* ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG VERSION=dev
RUN go build -trimpath \
      -ldflags="-s -w -X github.com/benerages/pixsheaf/internal/version.Version=${VERSION}" \
      -o /out/pixsheaf-worker ./cmd/pixsheaf-worker \
 && mkdir -p /out/run/pixsheaf

FROM ${RUNTIME_IMAGE}
COPY --from=build /out/pixsheaf-worker /usr/local/bin/pixsheaf-worker
# Same owner as in the API image; the worker mounts the volume read-only.
COPY --from=build --chown=10001:10001 --chmod=0770 /out/run/pixsheaf /run/pixsheaf
USER 10001:10001
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/pixsheaf-worker", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/pixsheaf-worker"]
