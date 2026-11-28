<script lang="ts">
  import { untrack } from "svelte";
  import { Batch, type Row } from "../batch.svelte";
  import type { Editor } from "../editor.svelte";
  import type { PickedFile } from "../files";
  import { changePercent, formatBytes, formatPercent } from "../format";
  import { errorText, numberLocale, t } from "../i18n/i18n.svelte";
  import type { MessageKey } from "../i18n/messages";
  import { baseName, type ExampleVars } from "../naming";
  import { Preview } from "../preview.svelte";
  import Compare from "./Compare.svelte";
  import RemovedNotice from "./RemovedNotice.svelte";
  import SettingsPanel from "./SettingsPanel.svelte";

  let {
    files = $bindable(),
    editor,
    formats = null,
    onreset,
  }: {
    files: PickedFile[];
    editor: Editor;
    formats?: readonly string[] | null;
    onreset: () => void;
  } = $props();

  const batch = new Batch();
  const preview = new Preview();
  // -1 until the effect below picks the first image: starting at 0 would
  // briefly preview a ZIP archive picked as the first file.
  let selected = $state(-1);

  // Only the picked files drive this effect; setFiles also reads the job
  // state, which must not restart the list once the batch runs.
  $effect(() => {
    const picked = files;
    untrack(() => {
      batch.setFiles(picked);
      selected = picked.findIndex((f) => !f.path.toLowerCase().endsWith(".zip"));
    });
  });
  $effect(() => () => {
    batch.reset();
    preview.reset();
  });

  const previewFile = $derived.by(() => {
    const file = selected >= 0 ? files[selected] : undefined;
    return file && !file.path.toLowerCase().endsWith(".zip") ? file : undefined;
  });
  const previewUrl = $derived(previewFile ? URL.createObjectURL(previewFile.file) : "");
  $effect(() => {
    const url = previewUrl;
    return () => {
      if (url) {
        URL.revokeObjectURL(url);
      }
    };
  });

  // Preview the selected image with the current settings until the batch
  // starts; the batch itself then shows the results per file.
  $effect(() => {
    const pipeline = $state.snapshot(editor.pipeline);
    const file = previewFile;
    if (!file || batch.started) {
      return;
    }
    const timer = setTimeout(() => void preview.run(file.file, file.path, pipeline), 600);
    return () => clearTimeout(timer);
  });

  // The previewed image's size drives the resize suggestions (see SingleView).
  // The size is kept with its URL, so a newly selected image never inherits
  // the previous one's size.
  let loaded = $state<{ url: string; size: [number, number] } | null>(null);
  const beforeSize = $derived(loaded?.url === previewUrl ? loaded.size : null);
  $effect(() => {
    const input = preview.input;
    editor.source = beforeSize
      ? { width: beforeSize[0], height: beforeSize[1] }
      : input?.width && input.height
        ? { width: input.width, height: input.height }
        : null;
  });

  const locale = $derived(numberLocale());

  // Name example for the previewed file, with its real output size.
  const example = $derived.by((): ExampleVars | null => {
    const out = preview.output;
    if (!previewFile || !out) {
      return null;
    }
    const index = batch.images.findIndex((r) => r.local === previewFile) + 1;
    return {
      name: baseName(previewFile.path),
      index: Math.max(index, 1),
      total: batch.images.length,
      width: out.width,
      height: out.height,
      format: out.format,
      date: new Date(),
    };
  });
  const total = $derived(batch.rows.reduce((sum, r) => sum + (r.local ? r.bytes : 0), 0));
  const images = $derived(batch.images.length);
  const done = $derived(batch.finished);
  const allDone = $derived(batch.jobStatus === "done");

  function statusKey(row: Row): MessageKey {
    if (row.file?.kind === "archive" && row.status === "done") {
      return "status.unpacked";
    }
    return `status.${row.status}` as MessageKey;
  }

  function start() {
    preview.cancel();
    void batch.start($state.snapshot(editor.pipeline), $state.snapshot(editor.naming));
  }
</script>

<div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
  <section class="min-w-0 space-y-4" aria-labelledby="batch-title">
    <div class="flex flex-wrap items-baseline justify-between gap-2">
      <h2 id="batch-title" class="text-xl font-semibold">
        {images === 1 ? t("batch.titleOne") : t("batch.title", { count: images })}
      </h2>
      <p class="muted text-sm tabular-nums">
        {t("batch.totalSize", { size: formatBytes(total, locale) })}
      </p>
    </div>

    {#if !batch.started && previewFile}
      <div class="space-y-2">
        <p class="text-sm font-medium">{t("batch.previewOf", { name: previewFile.path })}</p>
        <Compare
          before={previewUrl}
          after={preview.url}
          afterFormat={preview.output?.format ?? ""}
          working={preview.state === "working"}
          onbeforesize={(w, h) => (loaded = { url: previewUrl, size: [w, h] })}
        />
        {#if preview.state === "error"}
          <p class="text-sm text-red-700 dark:text-red-400" role="alert">
            {errorText(preview.error, preview.errorDetail)}
          </p>
        {/if}
      </div>
    {/if}

    {#if batch.started}
      <div aria-live="polite">
        <div class="mb-1 flex justify-between text-sm">
          <span>{t("batch.progress", { done, total: images })}</span>
        </div>
        <progress class="progress" max={Math.max(images, 1)} value={done}></progress>
      </div>
    {/if}

    <RemovedNotice names={batch.removed} {formats} ondismiss={() => batch.dismissRemoved()} />

    {#if batch.error}
      <p
        class="rounded-xl border border-red-300 bg-red-50 p-4 text-sm text-red-900 dark:border-red-900 dark:bg-red-950 dark:text-red-100"
        role="alert"
      >
        {errorText(batch.error)}
      </p>
    {/if}

    <div class="card overflow-x-auto">
      <table class="w-full text-left text-sm">
        <thead class="muted border-b border-stone-200 text-xs dark:border-stone-800">
          <tr>
            <th scope="col" class="px-3 py-2 font-medium">{t("batch.file")}</th>
            <th scope="col" class="hidden px-3 py-2 font-medium sm:table-cell">
              {t("batch.size")}
            </th>
            <th scope="col" class="px-3 py-2 font-medium">{t("batch.status")}</th>
            <th scope="col" class="hidden px-3 py-2 font-medium sm:table-cell">
              {t("batch.result")}
            </th>
            <th scope="col" class="px-3 py-2 font-medium">
              <span class="sr-only">{t("batch.actions")}</span>
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-stone-100 dark:divide-stone-800">
          {#each batch.rows as row, i (row.key)}
            <tr>
              <td class="max-w-40 truncate px-3 py-2 sm:max-w-64" title={row.name}>
                {row.name}
                <span class="muted block text-xs tabular-nums sm:hidden">
                  {formatBytes(row.bytes, locale)}
                  {#if row.file?.output}
                    → {formatBytes(row.file.output.bytes, locale)}
                  {/if}
                </span>
                {#if row.file?.archiveId}
                  <span class="muted block text-xs">
                    {t("batch.fromArchive", { name: batch.archiveName(row.file) })}
                  </span>
                {/if}
              </td>
              <td class="hidden px-3 py-2 tabular-nums sm:table-cell">
                {formatBytes(row.bytes, locale)}
              </td>
              <td class="px-3 py-2">
                <span
                  class={row.status === "failed"
                    ? "text-red-700 dark:text-red-400"
                    : row.status === "done"
                      ? "text-accent-700 dark:text-accent-300"
                      : ""}
                >
                  {t(statusKey(row))}
                </span>
                {#if row.error}
                  <span class="muted block text-xs">{errorText(row.error)}</span>
                {/if}
              </td>
              <td class="hidden px-3 py-2 tabular-nums sm:table-cell">
                {#if row.file?.output}
                  {formatBytes(row.file.output.bytes, locale)}
                  <span class="muted text-xs">
                    {formatPercent(changePercent(row.bytes, row.file.output.bytes), locale)}
                  </span>
                {/if}
              </td>
              <td class="px-3 py-2 text-right whitespace-nowrap">
                {#if !batch.started && row.local}
                  {#if !row.local.path.toLowerCase().endsWith(".zip")}
                    <button
                      type="button"
                      class="btn-quiet"
                      aria-pressed={selected === i}
                      onclick={() => (selected = i)}
                    >
                      {t("batch.preview")}
                    </button>
                  {/if}
                  <button
                    type="button"
                    class="btn-quiet"
                    aria-label={t("batch.remove", { name: row.name })}
                    onclick={() => (files = files.filter((f) => f !== row.local))}
                  >
                    ×
                  </button>
                {:else if row.status === "done" && row.file?.kind === "image" && !row.downloaded && !batch.zipDownloaded}
                  <a
                    class="btn-quiet"
                    href={batch.fileUrl(row)}
                    download
                    aria-label={t("batch.download", { name: row.file?.output?.name ?? row.name })}
                    onclick={() => batch.markDownloaded(row.key)}
                  >
                    ↓
                  </a>
                {:else if row.status === "queued" || row.status === "processing"}
                  <button
                    type="button"
                    class="btn-quiet"
                    aria-label={t("batch.cancel", { name: row.name })}
                    onclick={() => void batch.cancel(row)}
                  >
                    ×
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="flex flex-wrap items-center gap-3">
      {#if !batch.started}
        <button
          type="button"
          class="btn-primary"
          onclick={start}
          disabled={batch.rows.length === 0 || editor.patternProblem !== null}
        >
          {images === 1 ? t("batch.startOne") : t("batch.start", { count: images })}
        </button>
      {:else if allDone && !batch.zipDownloaded}
        <a
          class="btn-primary"
          href={batch.zipUrl()}
          download
          onclick={() => (batch.zipDownloaded = true)}
        >
          {t("batch.downloadAll")}
        </a>
      {:else if !allDone && batch.jobStatus !== "canceled"}
        <button type="button" class="btn-secondary" onclick={() => void batch.cancelAll()}>
          {t("batch.cancelAll")}
        </button>
      {/if}
      {#if batch.zipDownloaded}
        <p class="muted text-sm" role="status">{t("batch.downloaded")}</p>
      {/if}
      <button type="button" class="btn-secondary" onclick={onreset}>{t("batch.startOver")}</button>
    </div>
  </section>

  <aside class="lg:sticky lg:top-4 lg:self-start">
    <SettingsPanel {editor} naming {example} />
  </aside>
</div>
