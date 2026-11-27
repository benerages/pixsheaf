// Browser end-to-end and accessibility tests against a running stack
// (docker compose up). Run them with tests/e2e/ui.sh, which uses the
// pinned Playwright image so the browsers match the library version.
import { defineConfig, devices } from "@playwright/test";

const ci = process.env.CI === "true";
// The test container writes its results to a mounted directory.
const output = process.env.PW_OUTPUT;

export default defineConfig({
  testDir: "e2e",
  outputDir: output ? `${output}/results` : "test-results",
  timeout: 60_000,
  expect: { timeout: 20_000 },
  forbidOnly: ci,
  retries: 0,
  workers: 2,
  reporter: [
    ["list"],
    ["html", { open: "never", outputFolder: output ? `${output}/report` : "playwright-report" }],
  ],
  use: {
    baseURL: process.env.PIXSHEAF_URL ?? "http://127.0.0.1:8080",
    locale: "en-US",
    trace: "retain-on-failure",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    { name: "firefox", use: { ...devices["Desktop Firefox"] }, testIgnore: /a11y/ },
    { name: "mobile", use: { ...devices["Pixel 7"] }, testIgnore: /a11y/ },
  ],
});
