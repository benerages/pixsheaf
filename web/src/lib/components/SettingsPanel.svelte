<script lang="ts">
  import type { Editor } from "../editor.svelte";
  import { t } from "../i18n/i18n.svelte";
  import type { MessageKey } from "../i18n/messages";
  import {
    losslessFormats,
    lossyFormats,
    outputFormats,
    parsePipeline,
    type Aspect,
    type Gravity,
    type ResizeMode,
  } from "../pipeline";
  import { exportPreset, importPreset, presets, slug } from "../presets.svelte";

  let { editor, naming = false }: { editor: Editor; naming?: boolean } = $props();

  const s = $derived(editor.settings);
  const quick = $derived(editor.custom === null);
  const aspects: Aspect[] = ["none", "1:1", "4:3", "3:2", "16:9", "4:5"];
  const positions: Gravity[] = [
    "northwest",
    "north",
    "northeast",
    "west",
    "center",
    "east",
    "southwest",
    "south",
    "southeast",
  ];
  const resizeModes: [ResizeMode, MessageKey][] = [
    ["none", "settings.resizeNone"],
    ["width", "settings.resizeWidth"],
    ["height", "settings.resizeHeight"],
    ["longest", "settings.resizeLongest"],
    ["percent", "settings.resizePercent"],
  ];

  let pipelineText = $state("");
  let pipelineError = $state(false);
  let importError = $state(false);
  let importInput: HTMLInputElement | undefined = $state();

  // Show the current pipeline in the editor whenever it changes.
  $effect(() => {
    pipelineText = JSON.stringify(editor.pipeline, null, 2);
    pipelineError = false;
  });

  function choosePreset(id: string) {
    const preset = presets.find(id);
    if (preset) {
      editor.applyPreset(preset);
    } else {
      editor.useQuickSettings();
    }
  }

  function applyPipeline() {
    const p = parsePipeline(pipelineText);
    pipelineError = p === null;
    if (p) {
      editor.setCustom(p);
    }
  }

  function rotate(delta: 90 | -90) {
    s.rotate = (((s.rotate + delta) % 360) + 360) % 360 as 0 | 90 | 180 | 270;
  }

  function savePreset() {
    const title = window.prompt(t("settings.savePrompt"))?.trim();
    if (title) {
      const preset = editor.toPreset(slug(title), title.slice(0, 100));
      presets.save(preset);
      editor.applyPreset(preset);
    }
  }

  async function imported(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = "";
    if (!file) {
      return;
    }
    const preset = await importPreset(file);
    importError = preset === null;
    if (preset) {
      presets.save(preset);
      editor.applyPreset(preset);
    }
  }

  function exportCurrent() {
    const current = presets.find(editor.presetId);
    exportPreset(current ?? editor.toPreset("custom-settings", t("settings.presetNone")));
  }
</script>

<div class="card divide-y divide-stone-200 dark:divide-stone-800">
  <div class="p-4">
    <h2 class="text-base font-semibold">{t("settings.title")}</h2>
    <label class="label mt-3" for="preset">{t("settings.preset")}</label>
    <select
      id="preset"
      class="field"
      value={editor.presetId}
      onchange={(e) => choosePreset((e.currentTarget as HTMLSelectElement).value)}
    >
      <option value="">{t("settings.presetNone")}</option>
      {#each presets.bundled as p (p.id)}
        <option value={p.id}>{p.title}</option>
      {/each}
      {#each presets.custom as p (p.id)}
        <option value={p.id}>{p.title}</option>
      {/each}
    </select>
    {#if editor.presetId}
      <p class="muted mt-2 text-sm">{presets.find(editor.presetId)?.description ?? ""}</p>
    {/if}
    {#if !quick && !editor.presetId}
      <p class="muted mt-2 text-sm">{t("settings.custom")}</p>
    {/if}
  </div>

  <fieldset class="space-y-3 p-4" disabled={!quick}>
    <legend class="sr-only">{t("settings.format")}</legend>
    <div>
      <label class="label" for="format">{t("settings.format")}</label>
      <select id="format" class="field" bind:value={s.format}>
        {#each outputFormats as f (f)}
          <option value={f}>{f === "keep" ? t("settings.formatKeep") : f.toUpperCase()}</option>
        {/each}
      </select>
    </div>
    {#if losslessFormats.has(s.format)}
      <label class="flex items-center gap-2 text-sm">
        <input type="checkbox" bind:checked={s.lossless} class="size-4 accent-accent-700" />
        {t("settings.lossless")}
      </label>
    {/if}
    {#if !s.lossless || !losslessFormats.has(s.format)}
      <div>
        <span class="label" id="compression-label">{t("settings.compression")}</span>
        <div class="flex flex-wrap gap-x-4 gap-y-1 text-sm" role="radiogroup" aria-labelledby="compression-label">
          <label class="flex items-center gap-1.5">
            <input type="radio" value="auto" bind:group={s.compression} class="accent-accent-700" />
            {t("settings.compressionAuto")}
          </label>
          {#if s.format === "keep" || lossyFormats.has(s.format)}
            <label class="flex items-center gap-1.5">
              <input type="radio" value="quality" bind:group={s.compression} class="accent-accent-700" />
              {t("settings.compressionQuality")}
            </label>
          {/if}
          {#if !["gif", "tiff", "bmp"].includes(s.format)}
            <label class="flex items-center gap-1.5">
              <input type="radio" value="size" bind:group={s.compression} class="accent-accent-700" />
              {t("settings.compressionSize")}
            </label>
          {/if}
        </div>
      </div>
      {#if s.compression === "quality"}
        <div>
          <label class="label" for="quality">{t("settings.quality", { value: s.quality })}</label>
          <input id="quality" type="range" min="1" max="100" bind:value={s.quality} class="w-full accent-accent-700" />
        </div>
      {:else if s.compression === "size"}
        <div>
          <label class="label" for="target">{t("settings.targetKb")}</label>
          <input id="target" type="number" min="1" max="1048576" bind:value={s.targetKb} class="field" />
        </div>
      {/if}
    {/if}
  </fieldset>

  <fieldset class="space-y-3 p-4" disabled={!quick}>
    <legend class="sr-only">{t("settings.resize")}</legend>
    <div class={s.resize === "none" ? "" : "grid grid-cols-2 gap-3"}>
      <div>
        <label class="label" for="resize">{t("settings.resize")}</label>
        <select id="resize" class="field" bind:value={s.resize}>
          {#each resizeModes as [mode, key] (mode)}
            <option value={mode}>{t(key)}</option>
          {/each}
        </select>
      </div>
      {#if s.resize !== "none"}
        <div>
          <label class="label" for="resize-value">
            {s.resize === "percent" ? t("settings.resizeValuePercent") : t("settings.resizeValuePx")}
          </label>
          <input
            id="resize-value"
            type="number"
            min="1"
            max={s.resize === "percent" ? 100 : 65535}
            bind:value={s.resizeValue}
            class="field"
          />
        </div>
      {/if}
    </div>
    {#if s.resize !== "none"}
      <p class="muted text-xs">{t("settings.resizeNote")}</p>
    {/if}
    <div>
      <label class="label" for="crop">{t("settings.crop")}</label>
      <select id="crop" class="field" bind:value={s.crop}>
        {#each aspects as a (a)}
          <option value={a}>{a === "none" ? t("settings.cropNone") : a}</option>
        {/each}
      </select>
    </div>
    <div class="grid grid-cols-2 gap-3">
      <div>
        <span class="label">{t("settings.rotate")}</span>
        <div class="flex items-center gap-1">
          <button type="button" class="btn-secondary px-2" aria-label={t("settings.rotateLeft")} onclick={() => rotate(-90)}>
            <svg aria-hidden="true" viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 15 3 9m0 0 6-6M3 9h12a6 6 0 0 1 0 12h-3" /></svg>
          </button>
          <button type="button" class="btn-secondary px-2" aria-label={t("settings.rotateRight")} onclick={() => rotate(90)}>
            <svg aria-hidden="true" viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="m15 15 6-6m0 0-6-6m6 6H9a6 6 0 0 0 0 12h3" /></svg>
          </button>
          <span class="ml-1 text-sm tabular-nums" aria-live="polite">{t("settings.rotation", { angle: s.rotate })}</span>
        </div>
      </div>
      <div>
        <label class="label" for="flip">{t("settings.flip")}</label>
        <select id="flip" class="field" bind:value={s.flip}>
          <option value="none">{t("settings.flipNone")}</option>
          <option value="horizontal">{t("settings.flipHorizontal")}</option>
          <option value="vertical">{t("settings.flipVertical")}</option>
        </select>
      </div>
    </div>
  </fieldset>

  <fieldset class="space-y-3 p-4" disabled={!quick}>
    <legend class="sr-only">{t("settings.metadata")}</legend>
    <label class="flex items-start gap-2 text-sm">
      <input type="checkbox" bind:checked={s.keepMetadata} class="mt-0.5 size-4 accent-accent-700" />
      <span>
        {t("settings.metadata")}
        <span class="muted block text-xs">{t("settings.metadataNote")}</span>
      </span>
    </label>
    <label class="flex items-center gap-2 text-sm">
      <input type="checkbox" bind:checked={s.watermark} class="size-4 accent-accent-700" />
      {t("settings.watermark")}
    </label>
    {#if s.watermark}
      <div>
        <label class="label" for="wm-text">{t("settings.watermarkText")}</label>
        <input id="wm-text" type="text" maxlength="200" bind:value={s.watermarkText} class="field" />
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="label" for="wm-position">{t("settings.watermarkPosition")}</label>
          <select id="wm-position" class="field" bind:value={s.watermarkPosition}>
            {#each positions as p (p)}
              <option value={p}>{t(`position.${p}`)}</option>
            {/each}
          </select>
        </div>
        <div>
          <label class="label" for="wm-opacity">
            {t("settings.watermarkOpacity", { value: Math.round(s.watermarkOpacity * 100) })}
          </label>
          <input id="wm-opacity" type="range" min="0.05" max="1" step="0.05" bind:value={s.watermarkOpacity} class="w-full accent-accent-700" />
        </div>
      </div>
    {/if}
  </fieldset>

  {#if naming}
    <fieldset class="space-y-3 p-4">
      <legend class="text-sm font-semibold">{t("batch.naming")}</legend>
      <div>
        <label class="label" for="pattern">{t("batch.pattern")}</label>
        <input id="pattern" type="text" maxlength="200" bind:value={editor.pattern} class="field font-mono" aria-describedby="pattern-hint" />
        <p id="pattern-hint" class="muted mt-1 text-xs">{t("batch.patternHint")}</p>
      </div>
      <label class="flex items-center gap-2 text-sm">
        <input type="checkbox" bind:checked={editor.keepFolders} class="size-4 accent-accent-700" />
        {t("batch.keepFolders")}
      </label>
    </fieldset>
  {/if}

  <details class="p-4">
    <summary class="cursor-pointer text-sm font-medium">{t("settings.advanced")}</summary>
    <p class="muted mt-2 text-xs">{t("settings.advancedNote")}</p>
    <label class="sr-only" for="pipeline">{t("settings.advanced")}</label>
    <textarea
      id="pipeline"
      bind:value={pipelineText}
      rows="10"
      spellcheck="false"
      class="field mt-2 font-mono text-xs"
      aria-invalid={pipelineError}
      aria-describedby={pipelineError ? "pipeline-error" : undefined}
    ></textarea>
    {#if pipelineError}
      <p id="pipeline-error" class="mt-1 text-sm text-red-700 dark:text-red-400">{t("settings.advancedInvalid")}</p>
    {/if}
    <div class="mt-2 flex flex-wrap gap-2">
      <button type="button" class="btn-secondary" onclick={applyPipeline}>{t("settings.advancedApply")}</button>
      {#if !quick}
        <button type="button" class="btn-quiet" onclick={() => editor.useQuickSettings()}>
          {t("settings.advancedReset")}
        </button>
      {/if}
    </div>
    <div class="mt-4 flex flex-wrap gap-2 border-t border-stone-200 pt-4 dark:border-stone-800">
      <button type="button" class="btn-quiet" onclick={savePreset}>{t("settings.save")}</button>
      <button type="button" class="btn-quiet" onclick={exportCurrent}>{t("settings.export")}</button>
      <button type="button" class="btn-quiet" onclick={() => importInput?.click()}>{t("settings.import")}</button>
      {#if editor.presetId.startsWith("custom-")}
        <button
          type="button"
          class="btn-quiet"
          onclick={() => {
            presets.remove(editor.presetId);
            editor.useQuickSettings();
          }}>{t("settings.delete")}</button
        >
      {/if}
      <input bind:this={importInput} type="file" accept="application/json,.json" class="hidden" onchange={imported} />
    </div>
    {#if importError}
      <p class="mt-1 text-sm text-red-700 dark:text-red-400" role="alert">{t("settings.importInvalid")}</p>
    {/if}
  </details>
</div>
