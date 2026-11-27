<script lang="ts">
  import type { Editor } from "../editor.svelte";
  import type { PickedFile } from "../files";
  import { changePercent, formatBytes, formatPercent } from "../format";
  import { errorText, numberLocale, t } from "../i18n/i18n.svelte";
  import { Preview } from "../preview.svelte";
  import Compare from "./Compare.svelte";
  import SettingsPanel from "./SettingsPanel.svelte";

  let { picked, editor, onreset }: { picked: PickedFile; editor: Editor; onreset: () => void } = $props();

  const preview = new Preview();
  const beforeUrl = $derived(URL.createObjectURL(picked.file));
  let beforeSize = $state<[number, number] | null>(null);

  $effect(() => {
    const url = beforeUrl;
    return () => URL.revokeObjectURL(url);
  });

  // Process again shortly after the last change of the settings.
  $effect(() => {
    const pipeline = $state.snapshot(editor.pipeline);
    const file = picked;
    const timer = setTimeout(() => void preview.run(file.file, file.path, pipeline), 500);
    return () => clearTimeout(timer);
  });

  $effect(() => () => preview.reset());

  const out = $derived(preview.output);
  const locale = $derived(numberLocale());
</script>

<div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
  <section class="min-w-0 space-y-4" aria-label={picked.path}>
    <Compare
      before={beforeUrl}
      after={preview.url}
      afterFormat={out?.format ?? ""}
      working={preview.state === "working"}
      onbeforesize={(w, h) => (beforeSize = [w, h])}
    />

    <dl class="grid grid-cols-2 gap-4 text-sm">
      <div class="card p-3">
        <dt class="muted">{t("compare.before")}</dt>
        <dd class="mt-1 font-medium tabular-nums">
          {formatBytes(picked.file.size, locale)}
          {#if beforeSize}
            <span class="muted font-normal">· {t("info.dimensions", { w: beforeSize[0], h: beforeSize[1] })}</span>
          {/if}
        </dd>
      </div>
      <div class="card p-3">
        <dt class="muted">{t("compare.after")}</dt>
        <dd class="mt-1 font-medium tabular-nums" aria-live="polite">
          {#if out}
            {formatBytes(out.bytes, locale)}
            <span class="muted font-normal">· {t("info.dimensions", { w: out.width, h: out.height })}</span>
            <span class="muted block text-xs font-normal">
              {t("info.change", { percent: formatPercent(changePercent(picked.file.size, out.bytes), locale) })}
            </span>
          {:else}
            –
          {/if}
        </dd>
      </div>
    </dl>

    {#if preview.state === "error"}
      <div class="rounded-xl border border-red-300 bg-red-50 p-4 text-sm text-red-900 dark:border-red-900 dark:bg-red-950 dark:text-red-100" role="alert">
        {errorText(preview.error, preview.errorDetail)}
        <button
          type="button"
          class="btn-quiet ml-2 underline"
          onclick={() => void preview.run(picked.file, picked.path, $state.snapshot(editor.pipeline))}
        >
          {t("single.retry")}
        </button>
      </div>
    {/if}

    <div class="flex flex-wrap items-center gap-3">
      {#if preview.state === "done" && out}
        <a class="btn-primary" href={preview.downloadUrl} download={out.name}>
          <svg aria-hidden="true" viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4v12m0 0 4-4m-4 4-4-4M4 20h16" /></svg>
          {t("single.download", { name: out.name })}
        </a>
      {:else}
        <button type="button" class="btn-primary" disabled>{t("compare.working")}</button>
      {/if}
      <button type="button" class="btn-secondary" onclick={onreset}>{t("single.startOver")}</button>
    </div>
  </section>

  <aside class="lg:sticky lg:top-4 lg:self-start">
    <SettingsPanel {editor} />
  </aside>
</div>
