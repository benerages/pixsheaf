import { expect, test } from "@playwright/test";
import { fileInput, fixture, watchErrors } from "./helpers";

test("one image: process, compare, and download a single file", async ({ page, browserName }) => {
  const errors = watchErrors(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toContainText(
    "Convert, resize, and optimize images",
  );

  await fileInput(page).setInputFiles(fixture("gradient.png"));
  await expect(page.getByRole("link", { name: "Download gradient.png" })).toBeVisible();
  await expect(page.getByText(/batch/i)).toHaveCount(0);

  await page.getByLabel("Format", { exact: true }).selectOption("webp");
  const link = page.getByRole("link", { name: "Download gradient.webp" });
  await expect(link).toBeVisible();
  await expect(link).toHaveAttribute("download", "gradient.webp");
  await expect(link).toHaveAttribute("href", /^blob:/);
  // Playwright's Firefox does not report downloads of blob: links (the
  // batch ZIP, a server URL, does download there); the other browsers
  // check the real download.
  if (browserName !== "firefox") {
    const [download] = await Promise.all([page.waitForEvent("download"), link.click()]);
    expect(download.suggestedFilename()).toBe("gradient.webp");
  }

  // The comparison slider works with the keyboard.
  const slider = page.getByRole("slider", { name: "Comparison position" });
  await slider.focus();
  const start = await slider.inputValue();
  await page.keyboard.press("ArrowLeft");
  await expect(slider).not.toHaveValue(start);

  await page.getByRole("button", { name: "Choose another image" }).click();
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  expect(errors()).toEqual([]);
});

test("resize suggests half the size in percent and the original size in pixels", async ({
  page,
}) => {
  await page.goto("/");
  await fileInput(page).setInputFiles(fixture("gradient.png"));
  await expect(page.getByText("64 × 48 px").first()).toBeVisible();

  const mode = page.getByLabel("Resize", { exact: true });
  await mode.selectOption("percent");
  await expect(page.locator("#resize-value")).toHaveValue("50");
  await mode.selectOption("width");
  await expect(page.locator("#resize-value")).toHaveValue("64");
  await mode.selectOption("height");
  await expect(page.locator("#resize-value")).toHaveValue("48");
  await mode.selectOption("longest");
  await expect(page.locator("#resize-value")).toHaveValue("64");
});

test("a broken image shows a readable error", async ({ page }) => {
  await page.goto("/");
  await fileInput(page).setInputFiles(fixture("broken.png"));
  await expect(page.getByRole("alert")).toContainText("The image is damaged or could not be read.");
});

test.describe("German", () => {
  test.use({ locale: "de-CH" });

  test("the interface follows the browser language", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 1 })).toContainText("Bilder umwandeln");
    await page.getByLabel("Sprache").selectOption("en");
    await expect(page.getByRole("heading", { level: 1 })).toContainText("Convert");
  });
});
