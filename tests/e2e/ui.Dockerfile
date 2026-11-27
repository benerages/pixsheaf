# syntax=docker/dockerfile:1
# Browser test runner for tests/e2e/ui.sh: Playwright with Chromium and
# Firefox, plus axe-core. The image tag must match @playwright/test in
# web/package.json; scripts/pin-playwright-image.sh sets tag and digest,
# and Renovate keeps both in step afterwards (group "Playwright").
FROM mcr.microsoft.com/playwright:v1.65.1-noble@sha256:1507cbfc550a63a6b7396791ca4f00df17beb56342d2447b3f106c306f525579

# The image ships an LTS Node release, older than the >=26 the web build
# requires. The tests only run the Playwright CLI, so the engine check is
# relaxed for this install alone; the lockfile still applies.
ENV HOME=/tmp npm_config_ignore_scripts=true
RUN npm install --global pnpm@12.23.0
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile --config.engine-strict=false
COPY web/playwright.config.ts ./
COPY web/e2e/ e2e/
COPY tests/e2e/fixtures/ /src/tests/e2e/fixtures/

# Run as the image's unprivileged user: Firefox needs a real account with a
# home folder. Results go to /out and are copied out by ui.sh.
RUN install -d -o pwuser -g pwuser /out
USER pwuser
ENV HOME=/home/pwuser PW_OUTPUT=/out
CMD ["node_modules/.bin/playwright", "test"]
