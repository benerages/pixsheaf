import { expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

// Paths are relative to web/, where Playwright runs.
export const fixture = (name: string): string => `../tests/e2e/fixtures/${name}`;

/** The file input for images; the folder input has webkitdirectory. */
export const fileInput = (page: Page) =>
  page.locator('input[type="file"]:not([webkitdirectory]):not([accept*="json"])');

/**
 * Collects console errors and uncaught exceptions. Content Security Policy
 * violations appear as console errors, so an empty list also means the
 * strict CSP holds.
 */
export function watchErrors(page: Page): () => string[] {
  const errors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") {
      errors.push(message.text());
    }
  });
  page.on("pageerror", (error) => errors.push(error.message));
  return () => errors;
}

/** Fails with a readable list if axe finds WCAG 2.2 A or AA violations. */
export async function expectAccessible(page: Page): Promise<void> {
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"])
    .analyze();
  const violations = result.violations.map(
    (v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`,
  );
  expect(violations).toEqual([]);
}
