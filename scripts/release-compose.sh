#!/usr/bin/env bash
# Writes the compose file attached to a release: the repository's
# compose.yaml without build sections, with both images pinned to the
# release by tag and digest, so `docker compose up -d` pulls exactly the
# signed images and needs no source checkout.
#   scripts/release-compose.sh <version> <api-image@digest> <worker-image@digest> > compose.yaml
set -Eeuo pipefail
cd "$(dirname "$0")/.."
version=${1:?usage: $0 <version> <api-image@digest> <worker-image@digest>}
api=${2:?api image reference missing}
worker=${3:?worker image reference missing}
for ref in "$api" "$worker"; do
  [[ $ref =~ ^[a-z0-9./-]+:[0-9]+\.[0-9]+\.[0-9]+@sha256:[0-9a-f]{64}$ ]] ||
    { echo "not a tag@digest reference: $ref" >&2; exit 1; }
done

awk -v version="$version" -v api="$api" -v worker="$worker" '
  # Replace the header comment of the development file.
  NR == 1 {
    print "# Pixsheaf " version ": docker compose up -d"
    print "# Generated for the release; the images are pinned by digest and"
    print "# signed (see the release notes to verify them). The API listens on"
    print "# 127.0.0.1:8080 only; put a TLS reverse proxy in front of it to publish"
    print "# it. Settings: optional .env file next to this one, see"
    print "# https://github.com/benerages/pixsheaf/blob/v" version "/docs/configuration.md"
  }
  NR <= 3 && /^#/ { next }
  # Drop build sections: the "build:" line and its more indented lines.
  skip && match($0, /^ */) && RLENGTH > indent { next }
  skip = 0
  /^ *build:/ { match($0, /^ */); indent = RLENGTH; skip = 1; next }
  /^ *image: ghcr\.io\/[^\/]+\/pixsheaf-api:/    { sub(/image: .*/, "image: " api) }
  /^ *image: ghcr\.io\/[^\/]+\/pixsheaf-worker:/ { sub(/image: .*/, "image: " worker) }
  { print }
' compose.yaml
