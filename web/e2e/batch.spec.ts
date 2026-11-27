import { expect, test } from "@playwright/test";
import { fileInput, fixture, watchErrors } from "./helpers";

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
