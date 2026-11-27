# syntax=docker/dockerfile:1
# pixsheaf-api: Go binary with the embedded web UI on distroless (ADR 0005).
# Build from the repository root:
#   docker build -f deploy/docker/api.Dockerfile -t pixsheaf-api .
#
# Base images are pinned by digest where verified; Renovate pins and
# updates the remaining digests (ADR 0013).

ARG NODE_IMAGE=node:26.13.1-trixie-slim
ARG GO_IMAGE=golang:1.27.4-trixie@sha256:430985542100b871e108ccc00e8dcc5eef08ca14ee5825e98ebf563d8ffec4ee
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5

# --- Web UI ---------------------------------------------------------------
FROM ${NODE_IMAGE} AS web
ENV npm_config_ignore_scripts=true
RUN npm install --global pnpm@12.23.0
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# --- Go binary ------------------------------------------------------------
FROM ${GO_IMAGE} AS build
ENV GOTOOLCHAIN=local CGO_ENABLED=0
WORKDIR /src
COPY go.* ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/ui/dist/ internal/ui/dist/
ARG VERSION=dev
RUN go build -trimpath \
      -ldflags="-s -w -X github.com/benerages/pixsheaf/internal/version.Version=${VERSION}" \
      -o /out/pixsheaf-api ./cmd/pixsheaf-api \
 && mkdir -p /out/run/pixsheaf

# --- Runtime --------------------------------------------------------------
FROM ${RUNTIME_IMAGE}
COPY --from=build /out/pixsheaf-api /usr/local/bin/pixsheaf-api
# Socket directory for the worker channel; an empty named volume mounted
# here inherits this ownership (ADR 0007, amendment).
COPY --from=build --chown=10001:10001 --chmod=0770 /out/run/pixsheaf /run/pixsheaf
USER 10001:10001
ENV PIXSHEAF_LISTEN_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD ["/usr/local/bin/pixsheaf-api", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/pixsheaf-api"]
