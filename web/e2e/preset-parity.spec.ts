import { readFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";
import { fileInput, fixture } from "./helpers";

// Acceptance criterion: a preset used in the UI gives byte-identical
// results through the API for the same file.
test("a preset gives the same bytes in the UI and through the API", async ({
  page,
  request,
  browserName,
}) => {
  test.skip(browserName === "firefox", "Playwright's Firefox does not report blob downloads");

  // UI: apply the bundled Web preset and download the result.
  await page.goto("/");
  await fileInput(page).setInputFiles(fixture("gradient.png"));
  await page.getByLabel("Preset").selectOption("web");
  // The Web preset converts to WebP, so this link appears only for the
  // preview with the preset, not for the one before it.
  const link = page.getByRole("link", { name: "Download gradient.webp" });
  await expect(link).toBeVisible();
  const [download] = await Promise.all([page.waitForEvent("download"), link.click()]);
  const uiBytes = await readFile((await download.path()) ?? "");

  // API: the same preset document, the same file.
  const { presets } = (await (await request.get("/api/v1/presets")).json()) as {
    presets: { id: string; pipeline: unknown }[];
  };
  const web = presets.find((p) => p.id === "web");
  expect(web).toBeTruthy();
  const created = await request.post("/api/v1/jobs", {
    data: { pipeline: web?.pipeline, expectedFiles: 1 },
  });
  expect(created.status()).toBe(201);
  const { job, token } = (await created.json()) as { job: { id: string }; token: string };
  const auth = { Authorization: `Bearer ${token}` };
  const upload = await request.post(`/api/v1/jobs/${job.id}/files`, {
    headers: auth,
    multipart: {
      file: {
        name: "gradient.png",
        mimeType: "image/png",
        buffer: await readFile(fixture("gradient.png")),
      },
    },
  });
  expect(upload.status()).toBe(201);
  await expect
    .poll(
      async () =>
        (
          (await (await request.get(`/api/v1/jobs/${job.id}`, { headers: auth })).json()) as {
            status: string;
          }
        ).status,
    )
    .toBe("done");
  const result = await request.get(`/api/v1/jobs/${job.id}/download`, { headers: auth });
  expect(result.status()).toBe(200);
  const apiBytes = await result.body();

  expect(apiBytes.length).toBeGreaterThan(0);
  expect(Buffer.compare(uiBytes, apiBytes)).toBe(0);
});
