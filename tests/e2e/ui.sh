#!/usr/bin/env bash
# Browser end-to-end and accessibility tests (Playwright, axe-core) against a
# running stack:
#   docker compose up -d --build --wait && tests/e2e/ui.sh [playwright args]
# PIXSHEAF_URL overrides the address; the HTML report lands in
# web/test-results/report, traces of failed tests in web/test-results/results.
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
name="pixsheaf-ui-tests-$$"
trap 'docker rm --force "$name" >/dev/null 2>&1 || true' EXIT
status=0
docker run --name "$name" --network host --shm-size 1g \
  --env CI="${CI:-false}" \
  --env PIXSHEAF_URL="${PIXSHEAF_URL:-http://127.0.0.1:8080}" \
  pixsheaf-ui-tests node_modules/.bin/playwright test "$@" || status=$?

# Copy the report and traces out of the container (no bind mount, so the
# container user needs no access to the checkout).
rm -rf web/test-results
mkdir -p web/test-results
docker cp "$name:/out/." web/test-results/ >/dev/null || true
exit "$status"
