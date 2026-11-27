# Pixsheaf Phase 5: Web UI

Phase 5 is complete. The start page is the single-image workflow: drop, choose, or paste an image, pick an action or a preset, compare before and after with a slider, and download one file. Selecting several files, a folder, or a ZIP archive switches to batch mode without any extra step. The UI is a Svelte 5 single-page app embedded in the API binary; it uses only the public API, in English and German, light and dark, under the strict CSP. Browser end-to-end and accessibility tests run in CI.

## 1. What is in place

| Area | State |
|---|---|
| Single image | Drag & drop, file picker, clipboard paste; live preview after each change (debounced one-file job, result kept as a blob, the job deleted after the download); before-and-after slider (native range input, keyboard operable); size, dimensions, and change in percent; download as one file with the output name |
| Batch mode | Starts automatically for more than one file, a folder, or a ZIP archive; preview of a selected file before the start; three parallel uploads; per-file status over Server-Sent Events; cancel one file or all; remove files before the start; per-file download or the streamed ZIP; compact table on narrow screens |
| Settings | Format, quality or target size, lossless, resize (width, height, longest edge, percent; never enlarged), crop to aspect ratio, rotate, flip, keep metadata (location always removed), text watermark with position and opacity; "keep format" with compression becomes one conditional convert step per source format |
| Pipeline editor | The pipeline JSON (ADR 0009) is the single source of truth; the advanced editor shows it, applies edits, and returns to the quick settings |
| Presets | Bundled presets from `GET /presets`; custom presets saved in the browser (`localStorage`), exported and imported as the same JSON that `POST /jobs` accepts; while a preset is active, a note explains why the quick settings are locked |
| Naming | Pattern field with the server's rules mirrored in the UI (unknown placeholders, unmatched braces, length), an example for the previewed file with its real output size, and the start button disabled while the pattern is invalid |
| API addition | `GET /jobs/{jobId}/files/{fileId}/download`: one-time download of a single result of a batch (`JobFile.downloaded`); the job's ZIP then leaves that file out |
| i18n | English by default, German when the browser prefers it or the user chooses it; all texts and error codes in both languages (the type of the German dictionary enforces completeness) |
| Look | Calm and reduced, petrol accent, light and dark following `prefers-color-scheme`, reduced motion respected; component classes as Tailwind `@utility` |
| CSP | No inline styles or scripts: dynamic values are set through the CSSOM (`style.setProperty`), the ESLint rule `svelte/no-inline-styles` guards it, and the browser tests fail on any console error, which includes CSP violations |
| Size | JavaScript 115 kB (40 kB gzip), CSS 25 kB (5.5 kB gzip) |

## 2. Tests

| Kind | Coverage |
|---|---|
| Static | `svelte-check --fail-on-warnings`, ESLint (typescript-eslint strict, Svelte rules), Prettier |
| Browser end to end | `tests/e2e/ui.sh` runs Playwright 1.65.1 in the pinned Playwright image against the running stack, in Chromium, Firefox, and a Pixel 7 profile: single-image flow with format change, download name, keyboard slider, and no console errors; a broken image with a readable error; the German interface; batch mode with the pattern check, a broken file failing alone, the ZIP download, and removing files before the start |
| Accessibility | axe-core 4.14 (`@axe-core/playwright` 4.14.0) with the WCAG 2.0, 2.1, and 2.2 A and AA rules on the start page, the single-image view, and the batch view after processing, in light and dark: no violations |
| API | `TestDownloadSingleFileOfBatch` for the new per-file download; the shell end-to-end tests from Phases 3 and 4 still pass |
| Manual | The UI was checked in a browser against the stack: layout on desktop and phone width, both color schemes, a batch with a broken file, saving a preset with the keyboard |

The test runner image (`tests/e2e/ui.Dockerfile`) is pinned by tag and digest; `scripts/pin-playwright-image.sh` derives both from `@playwright/test`, `ui.sh` refuses a mismatch, and Renovate updates library and image together. The image ships an LTS Node release, so the engine check (Node 26 or later) is relaxed for that one install; the lockfile still applies. The tests run as the image's unprivileged user, and the report is copied out of the container on failure (CI artifact `playwright-report`).

## 3. Issues found and fixed

| Issue | Fix |
|---|---|
| Tailwind 4 rejects `@apply` of classes defined in `@layer components` | Component classes are `@utility` |
| Starting a batch reset the file list: the effect that loads the picked files also read the job state | The effect tracks only the picked files (`untrack`) |
| A ZIP archive counted as an image before the start | Archives are excluded from the image count |
| Size labels mixed decimal units with a limit configured in MiB ("26.2 MB" for 25 MiB) | Binary units with the familiar labels |
| Chromium recolored the native progress bar in dark mode | Themed progress bar |
| Firefox could open a downloaded image instead of saving it | The download link uses a separate blob of type `application/octet-stream` |
| Generated TypeScript types make fields with defaults required | The UI sends `colorSpace`, `mode`, `enlarge`, and `keepLocation` explicitly |

## 4. Known gaps

| Gap | Phase |
|---|---|
| Playwright's Firefox does not report downloads of `blob:` links; in Firefox the test checks the link (blob URL and file name) instead of the download, while Chromium and the phone profile check the real download | Revisit with newer Playwright versions |
| Prettier does not format `.svelte` files (no Svelte plugin yet) | 6 |
| Screen reader test by hand (NVDA, VoiceOver) and a full keyboard walkthrough of every control | 6 |
| Mozilla Observatory grade for the security headers | 6 |
| Coverage, load test with real photos and in CI, seccomp allowlist, gVisor (from earlier phases) | 6 |

## 5. Next: Phase 6

Hardening and release: review against OWASP ASVS 5.0 Level 2, the remaining tests and measurements above, complete documentation (README quick start, `docs/api.md` from OpenAPI, threat model update), and the signed 1.0 release with SBOM and provenance.
