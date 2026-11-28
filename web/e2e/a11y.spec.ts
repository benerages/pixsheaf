import { expect, test } from "@playwright/test";
import { expectAccessible, fileInput, fixture } from "./helpers";

// WCAG 2.2 AA checks with axe-core in both color schemes.
for (const colorScheme of ["light", "dark"] as const) {
  test.describe(`${colorScheme} scheme`, () => {
    test.use({ colorScheme });

    test("start page", async ({ page }) => {
      await page.goto("/");
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      await expect(page.getByRole("contentinfo")).toContainText("Version");
      await expectAccessible(page);
    });

    test("single-image view", async ({ page }) => {
      await page.goto("/");
      await fileInput(page).setInputFiles(fixture("gradient.png"));
      await expect(page.getByRole("link", { name: "Download gradient.png" })).toBeVisible();
      await expectAccessible(page);
    });

    test("batch view after processing", async ({ page }) => {
      await page.goto("/");
      await fileInput(page).setInputFiles([fixture("gradient.png"), fixture("broken.png")]);
      await page.getByRole("button", { name: "Process 2 files" }).click();
      await expect(page.getByRole("link", { name: "Download all as ZIP" })).toBeVisible();
      await expectAccessible(page);
    });
  });
}
