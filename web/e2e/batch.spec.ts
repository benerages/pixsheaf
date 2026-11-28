import { expect, test } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { fileInput, fixture, storedZip, watchErrors } from "./helpers";

const pdfBytes = Buffer.from("%PDF-1.7\n");

/**
 * Writes a generated test file to the test's output folder. Real paths are
 * needed: Playwright cannot mix paths with buffers, and Chromium hands
 * buffers to the page through blob: URLs that the strict CSP blocks.
 */
function tempFile(name: string, data: Buffer): string {
  const path = test.info().outputPath(name);
  writeFileSync(path, data);
  return path;
}

test("several files switch to batch mode and download as ZIP", async ({ page }) => {
  const errors = watchErrors(page);
  await page.goto("/");
  await fileInput(page).setInputFiles([fixture("gradient.png"), fixture("broken.png")]);
  await expect(page.getByRole("heading", { name: "2 files" })).toBeVisible();

  // The name pattern is checked while typing, with an example.
  const pattern = page.getByLabel("Name pattern");
  const start = page.getByRole("button", { name: "Process 2 files" });
  await pattern.fill("{name}-{size}");
  await expect(page.getByText("Unknown placeholder {size}.")).toBeVisible();
  await expect(start).toBeDisabled();
  await pattern.fill("{name}-{index}");
  await expect(page.getByText("Example: gradient-1.png")).toBeVisible();

  await start.click();
  const rows = page.getByRole("row");
  await expect(rows.filter({ hasText: "gradient" })).toContainText("Done");
  await expect(rows.filter({ hasText: "broken" })).toContainText("Failed");
  await expect(page.getByText("2 of 2 done")).toBeVisible();

  const zip = page.getByRole("link", { name: "Download all as ZIP" });
  const [download] = await Promise.all([page.waitForEvent("download"), zip.click()]);
  expect(download.suggestedFilename()).toMatch(/\.zip$/);
  await expect(page.getByText("Downloaded. The files were deleted from the server.")).toBeVisible();
  expect(errors()).toEqual([]);
});

test("removing files before the start keeps the list in sync", async ({ page }) => {
  await page.goto("/");
  await fileInput(page).setInputFiles([fixture("gradient.png"), fixture("broken.png")]);
  await page.getByRole("button", { name: "Remove broken.png" }).click();
  await expect(page.getByRole("heading", { name: "1 file" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Process 1 file" })).toBeEnabled();
});

test("unsupported files are left out of a selection and listed", async ({ page }) => {
  await page.goto("/");
  await fileInput(page).setInputFiles([
    fixture("gradient.png"),
    fixture("broken.png"),
    tempFile("notes.pdf", pdfBytes),
  ]);
  await expect(page.getByRole("heading", { name: "2 files" })).toBeVisible();
  const notice = page.getByRole("status").filter({ hasText: "left out" });
  await expect(notice).toContainText("1 file was left out because its format is not supported");
  await expect(notice).toContainText("notes.pdf");
  await expect(page.getByRole("row").filter({ hasText: "notes.pdf" })).toHaveCount(0);
  await notice.getByRole("button", { name: "Dismiss notice" }).click();
  await expect(notice).toHaveCount(0);
});

test("a selection without supported files stays on the start page", async ({ page }) => {
  await page.goto("/");
  await fileInput(page).setInputFiles([tempFile("notes.pdf", pdfBytes)]);
  await expect(page.getByRole("status").filter({ hasText: "left out" })).toContainText("notes.pdf");
  await expect(page.getByRole("button", { name: "Choose image" })).toBeVisible();
});

test("unsupported entries of a ZIP archive are left out and listed", async ({ page }) => {
  const errors = watchErrors(page);
  await page.goto("/");
  const zip = storedZip([
    { name: "photos/gradient.png", data: readFileSync(fixture("gradient.png")) },
    { name: "photos/notes.pdf", data: pdfBytes },
  ]);
  await fileInput(page).setInputFiles([tempFile("photos.zip", zip)]);
  await page.getByRole("button", { name: /^Process/ }).click();

  const rows = page.getByRole("row");
  await expect(rows.filter({ hasText: "gradient" })).toContainText("Done");
  const notice = page.getByRole("status").filter({ hasText: "left out" });
  await expect(notice).toContainText("notes.pdf");
  await expect(rows.filter({ hasText: "notes.pdf" })).toHaveCount(0);
  expect(errors()).toEqual([]);
});
