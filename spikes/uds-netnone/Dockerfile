# syntax=docker/dockerfile:1
# Spike image for ADR 0007 (job transport). Not a production image.
# Versions follow the Phase 1 report; digests are pinned in the Phase 2
# scaffold, not in this throwaway spike.

ARG GO_IMAGE=golang:1.27.4-trixie
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian13:nonroot

FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/spike main.go \
 && mkdir -p /out/sockdir

FROM ${RUNTIME_IMAGE}
COPY --from=build /out/spike /spike
# Socket directory owned by the service UID. An empty named volume mounted
# here inherits this ownership and mode (Docker "copy-up").
COPY --from=build --chown=10001:10001 --chmod=0770 /out/sockdir /run/pixsheaf
USER 10001:10001
ENTRYPOINT ["/spike"]
