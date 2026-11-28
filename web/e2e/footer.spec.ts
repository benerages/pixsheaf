import { expect, test } from "@playwright/test";

test("footer shows the server version and links to the repository", async ({ page }) => {
  await page.goto("/");
  const footer = page.getByRole("contentinfo");
  await expect(footer).toContainText(/Version (v\d+\.\d+\.\d+|dev)/);
  const link = footer.getByRole("link", { name: "Source code on GitHub (opens in a new tab)" });
  await expect(link).toHaveAttribute("href", "https://github.com/benerages/pixsheaf");
  await expect(link).toHaveAttribute("rel", "noopener noreferrer");
});
