import { expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { crc32 } from "node:zlib";

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

/** A minimal ZIP archive with stored (uncompressed) entries, built in memory. */
export function storedZip(entries: { name: string; data: Buffer }[]): Buffer {
  const parts: Buffer[] = [];
  const central: Buffer[] = [];
  let offset = 0;
  for (const { name, data } of entries) {
    const fileName = Buffer.from(name, "utf8");
    const crc = crc32(data);
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4); // version needed
    local.writeUInt16LE(0x0800, 6); // UTF-8 names
    local.writeUInt32LE(crc, 14);
    local.writeUInt32LE(data.length, 18);
    local.writeUInt32LE(data.length, 22);
    local.writeUInt16LE(fileName.length, 26);
    const header = Buffer.alloc(46);
    header.writeUInt32LE(0x02014b50, 0);
    header.writeUInt16LE(20, 4); // version made by
    header.writeUInt16LE(20, 6);
    header.writeUInt16LE(0x0800, 8);
    header.writeUInt32LE(crc, 16);
    header.writeUInt32LE(data.length, 20);
    header.writeUInt32LE(data.length, 24);
    header.writeUInt16LE(fileName.length, 28);
    header.writeUInt32LE(offset, 42);
    parts.push(local, fileName, data);
    central.push(header, fileName);
    offset += local.length + fileName.length + data.length;
  }
  const directory = Buffer.concat(central);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(directory.length, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...parts, directory, end]);
}
