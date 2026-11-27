# syntax=docker/dockerfile:1
# Browser test runner for tests/e2e/ui.sh: Playwright with Chromium and
# Firefox, plus axe-core. The image tag must match @playwright/test in
# web/package.json; scripts/pin-playwright-image.sh sets tag and digest,
# and Renovate keeps both in step afterwards (group "Playwright").
FROM mcr.microsoft.com/playwright:vUNPINNED-noble

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
CMD ["pnpm", "exec", "playwright", "test"]
