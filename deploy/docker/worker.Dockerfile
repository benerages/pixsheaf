# syntax=docker/dockerfile:1
# pixsheaf-worker: isolated image worker (ADRs 0006, 0008, 0016, 0017, 0018).
#
# Build stages run on Wolfi with every package pinned in
# deploy/docker/wolfi/build.lock. libvips, libheif, libde265, cgif, and quantizr are
# built from upstream tags whose commits are verified. The runtime image is
# assembled from scratch: only the shared libraries the worker binary needs,
# installed with apk so that image scanners see their versions. It has no
# shell and no package manager.
#
# Build from the repository root:
#   docker build -f deploy/docker/worker.Dockerfile -t pixsheaf-worker .
# Run the Go tests against libvips:
#   docker build -f deploy/docker/worker.Dockerfile --target test .
# Refresh all pins (base digest, source commits, package lock):
#   scripts/update-worker-pins.sh

ARG GO_IMAGE=golang:1.27.4-trixie@sha256:430985542100b871e108ccc00e8dcc5eef08ca14ee5825e98ebf563d8ffec4ee
ARG WOLFI_IMAGE=cgr.dev/chainguard/wolfi-base:latest@sha256:4cf5932a0f95a88af352f77e4efa67480cab0e1f7a48f0eddca3cce003a7c0fb

FROM ${GO_IMAGE} AS go

# --- Pinned Wolfi build environment -----------------------------------------
FROM ${WOLFI_IMAGE} AS deps
COPY deploy/docker/wolfi/build.lock /tmp/build.lock
# Versions come from the lock file, which hadolint cannot see.
# hadolint ignore=DL3018,SC2046
RUN grep -qv '^#' /tmp/build.lock || { echo "build.lock is empty; run scripts/update-worker-pins.sh" >&2; exit 1; } \
 && apk add --no-cache $(grep -v '^#' /tmp/build.lock)
COPY --chmod=0755 deploy/docker/fetch-source.sh /usr/local/bin/fetch-source
COPY --chmod=0755 deploy/docker/make-rootfs.sh /usr/local/bin/make-rootfs

# --- Native libraries from pinned source ------------------------------------
FROM deps AS native
ENV PKG_CONFIG_PATH=/usr/local/lib/pkgconfig \
    LD_LIBRARY_PATH=/usr/local/lib
WORKDIR /build

# Palette quantization for PNG and GIF output; MIT instead of
# libimagequant (ADR 0017).
# renovate: datasource=github-tags depName=DarthSim/quantizr
ARG QUANTIZR_VERSION=v1.4.3
ARG QUANTIZR_COMMIT=cfb26aaf3039ac1179d42a66cc7988c8c6feeba9
RUN fetch-source https://github.com/DarthSim/quantizr.git "$QUANTIZR_VERSION" "$QUANTIZR_COMMIT" quantizr
WORKDIR /build/quantizr
RUN cargo cinstall --release --library-type cdylib --prefix /usr/local --libdir /usr/local/lib
WORKDIR /build

# GIF encoder.
# renovate: datasource=github-tags depName=dloebl/cgif
ARG CGIF_VERSION=v0.5.4
ARG CGIF_COMMIT=2dfb49c22fbe3894e46731f7e3b6ffa84311e44e
RUN fetch-source https://github.com/dloebl/cgif.git "$CGIF_VERSION" "$CGIF_COMMIT" cgif \
 && meson setup cgif/build cgif --prefix=/usr/local --libdir=lib --buildtype=release \
 && meson install -C cgif/build

# HEVC decoder for HEIC input, the riskiest decoder in the image (ADR 0016).
# Built from source: Wolfi's libde265-dev 1.1.3 ships 1.1.3 headers but an
# older libde265.so.0.1.9 without the decoder security limits (ADR 0018).
# renovate: datasource=github-tags depName=strukturag/libde265
ARG LIBDE265_VERSION=v1.1.3
ARG LIBDE265_COMMIT=ba62bf4cfb3242f3bf0a45617ff09e35236e4d82
RUN fetch-source https://github.com/strukturag/libde265.git "$LIBDE265_VERSION" "$LIBDE265_COMMIT" libde265 \
 && cmake -S libde265 -B libde265/build \
      -DCMAKE_BUILD_TYPE=Release \
      -DCMAKE_INSTALL_PREFIX=/usr/local \
      -DCMAKE_INSTALL_LIBDIR=lib \
      -DBUILD_SHARED_LIBS=ON \
      -DENABLE_DECODER=OFF \
      -DENABLE_ENCODER=OFF \
      -DENABLE_SDL=OFF \
      -DENABLE_SHERLOCK265=OFF \
 && cmake --build libde265/build --parallel "$(nproc)" \
 && cmake --install libde265/build

# HEIF/AVIF with only the codecs Pixsheaf needs (ADR 0016): libde265 for
# HEIC decoding, dav1d for AVIF decoding, SVT-AV1 for AVIF encoding. No
# HEVC encoder, no plugin loading.
# renovate: datasource=github-tags depName=strukturag/libheif
ARG LIBHEIF_VERSION=v1.23.7
ARG LIBHEIF_COMMIT=12a259a5882dd99d45ec5fc77e296a3b620d1e2e
RUN fetch-source https://github.com/strukturag/libheif.git "$LIBHEIF_VERSION" "$LIBHEIF_COMMIT" libheif \
 && cmake -S libheif -B libheif/build \
      -DCMAKE_BUILD_TYPE=Release \
      -DCMAKE_INSTALL_PREFIX=/usr/local \
      -DCMAKE_INSTALL_LIBDIR=lib \
      -DBUILD_TESTING=OFF \
      -DWITH_EXAMPLES=OFF \
      -DWITH_GDK_PIXBUF=OFF \
      -DENABLE_PLUGIN_LOADING=OFF \
      -DWITH_LIBDE265=ON -DWITH_LIBDE265_PLUGIN=OFF \
      -DWITH_DAV1D=ON -DWITH_DAV1D_PLUGIN=OFF \
      -DWITH_SvtEnc=ON -DWITH_SvtEnc_PLUGIN=OFF \
      -DWITH_X265=OFF \
      -DWITH_AOM_DECODER=OFF -DWITH_AOM_ENCODER=OFF \
      -DWITH_RAV1E=OFF \
      -DWITH_JPEG_DECODER=OFF -DWITH_JPEG_ENCODER=OFF \
      -DWITH_OpenJPEG_DECODER=OFF -DWITH_OpenJPEG_ENCODER=OFF \
      -DWITH_OPENJPH_DECODER=OFF -DWITH_OPENJPH_ENCODER=OFF \
      -DWITH_OpenH264_DECODER=OFF -DWITH_OpenH264_ENCODER=OFF \
      -DWITH_FFMPEG_DECODER=OFF \
      -DWITH_KVAZAAR=OFF \
      -DWITH_UNCOMPRESSED_CODEC=OFF \
 && cmake --build libheif/build --parallel "$(nproc)" \
 && cmake --install libheif/build

# libvips with an explicit feature list: every loader and saver outside
# the allowlist (ADR 0008) is not compiled in at all. Highway (SIMD only,
# no parsing) is used if found: Wolfi's libjxl-dev ships its own libhwy and
# conflicts with highway-dev.
# renovate: datasource=github-tags depName=libvips/libvips
ARG LIBVIPS_VERSION=v8.18.7
ARG LIBVIPS_COMMIT=24ad4d042940e6bf99a68871ba886ca8847c9c82
RUN fetch-source https://github.com/libvips/libvips.git "$LIBVIPS_VERSION" "$LIBVIPS_COMMIT" libvips \
 && meson setup libvips/build libvips --prefix=/usr/local --libdir=lib --buildtype=release \
      -Dmodules=disabled -Dintrospection=disabled -Dvapi=false \
      -Dexamples=false -Dcplusplus=false -Ddocs=false \
      -Dcgif=enabled -Dexif=enabled -Dheif=enabled -Djpeg=enabled \
      -Djpeg-xl=enabled -Dlcms=enabled -Dpng=enabled -Dquantizr=enabled \
      -Dtiff=enabled -Dwebp=enabled -Dzlib=enabled -Dhighway=auto \
      -Dspng=disabled -Dimagequant=disabled -Darchive=disabled \
      -Dcfitsio=disabled -Dfftw=disabled -Dfontconfig=disabled \
      -Dmagick=disabled -Dmatio=disabled -Dnifti=disabled \
      -Dopenexr=disabled -Dopenjpeg=disabled -Dopenslide=disabled \
      -Dorc=disabled -Dpangocairo=disabled -Dpdfium=disabled \
      -Dpoppler=disabled -Draw=disabled -Drsvg=disabled -Duhdr=disabled \
      -Dnsgif=true -Dppm=false -Danalyze=false -Dradiance=false \
 && meson install -C libvips/build

# --- Worker binary (cgo against libvips) ------------------------------------
FROM native AS build
COPY --from=go /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH \
    GOTOOLCHAIN=local \
    CGO_ENABLED=1 \
    GOFLAGS=-tags=vips
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

# --- Tests against the real libvips build -----------------------------------
FROM build AS test
ENV SVT_LOG=1
RUN go vet ./... \
 && go test -count=1 ./...

# --- Minimal runtime root filesystem ----------------------------------------
FROM build AS rootfs
RUN make-rootfs /out/pixsheaf-worker /rootfs

FROM scratch
COPY --from=rootfs /rootfs/ /
# Same owner as in the API image; the worker mounts the volume read-only.
COPY --from=build --chown=10001:10001 --chmod=0700 /out/run/pixsheaf /run/pixsheaf
# SVT_LOG=1: SVT-AV1 logs errors only (it prints its configuration on
# every AVIF encode otherwise).
ENV LD_LIBRARY_PATH=/usr/local/lib \
    VIPS_BLOCK_UNTRUSTED=1 \
    SVT_LOG=1
USER 10001:10001
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/usr/local/bin/pixsheaf-worker", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/pixsheaf-worker"]
