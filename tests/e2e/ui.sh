#!/usr/bin/env bash
# Browser end-to-end and accessibility tests (Playwright, axe-core) against a
# running stack:
#   docker compose up -d --build --wait && tests/e2e/ui.sh [playwright args]
# PIXSHEAF_URL overrides the address; reports land in web/playwright-report
# and web/test-results.
set -Eeuo pipefail
cd "$(dirname "$0")/../.."

dockerfile=tests/e2e/ui.Dockerfile
image_ref=$(sed -n 's/^FROM //p' "$dockerfile")
if [[ $image_ref != *@sha256:* ]]; then
  echo "$dockerfile is not pinned; run scripts/pin-playwright-image.sh" >&2
  exit 1
fi
want=$(sed -n 's/.*"@playwright\/test": "\([^"]*\)".*/\1/p' web/package.json)
if [[ $image_ref != *":v${want}-"* ]]; then
  echo "Playwright image ($image_ref) does not match @playwright/test $want" >&2
  echo "run scripts/pin-playwright-image.sh" >&2
  exit 1
fi

docker build --quiet --file "$dockerfile" --tag pixsheaf-ui-tests . >/dev/null
mkdir -p web/playwright-report web/test-results
docker run --rm --network host --shm-size 1g \
  --user "$(id -u):$(id -g)" \
  --env CI="${CI:-false}" \
  --env PIXSHEAF_URL="${PIXSHEAF_URL:-http://127.0.0.1:8080}" \
  --env PW_OUTPUT=/out \
  --volume "$PWD/web/playwright-report:/out/report" \
  --volume "$PWD/web/test-results:/out/results" \
  pixsheaf-ui-tests pnpm exec playwright test "$@"
