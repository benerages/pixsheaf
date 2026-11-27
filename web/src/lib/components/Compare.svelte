<script lang="ts">
  import { t } from "../i18n/i18n.svelte";

  let {
    before,
    after = "",
    afterFormat = "",
    working = false,
    onbeforesize,
  }: {
    before: string;
    after?: string;
    afterFormat?: string;
    working?: boolean;
    onbeforesize?: (w: number, h: number) => void;
  } = $props();

  let position = $state(50);
  let frame: HTMLDivElement | undefined = $state();
  let beforeBroken = $state(false);
  let afterBroken = $state(false);
  let ratio = $state("4 / 3");

  // CSSOM changes are allowed by the CSP; inline style attributes are not.
  $effect(() => {
    frame?.style.setProperty("--pos", `${position}%`);
    frame?.style.setProperty("aspect-ratio", ratio);
  });
  $effect(() => {
    void before;
    beforeBroken = false;
  });
  $effect(() => {
    void after;
    afterBroken = false;
  });

  function loadedBefore(e: Event) {
    const img = e.currentTarget as HTMLImageElement;
    if (img.naturalWidth > 0) {
      ratio = `${img.naturalWidth} / ${img.naturalHeight}`;
      onbeforesize?.(img.naturalWidth, img.naturalHeight);
    }
  }
</script>

<div
  bind:this={frame}
  class="compare relative max-h-[70vh] w-full overflow-hidden rounded-2xl bg-stone-200 dark:bg-stone-800"
>
  {#if beforeBroken}
    <p class="muted absolute inset-0 flex items-center justify-center p-6 text-center text-sm">
      {t("compare.noPreview")}
    </p>
  {:else}
    <img
      src={before}
      alt={t("compare.before")}
      class="absolute inset-0 size-full object-contain"
      onload={loadedBefore}
      onerror={() => (beforeBroken = true)}
    />
  {/if}

  {#if after}
    <div class="compare-after absolute inset-0 bg-stone-200 dark:bg-stone-800">
      {#if afterFormat === "pdf"}
        <p class="absolute inset-0 flex items-center justify-center text-sm font-medium">
          {t("compare.pdf")}
        </p>
      {:else if afterBroken}
        <p class="muted absolute inset-0 flex items-center justify-center p-6 text-center text-sm">
          {t("compare.noPreview")}
        </p>
      {:else}
        <img
          src={after}
          alt={t("compare.after")}
          class="absolute inset-0 size-full object-contain"
          onerror={() => (afterBroken = true)}
        />
      {/if}
    </div>
    <div class="compare-handle pointer-events-none absolute inset-y-0" aria-hidden="true">
      <div class="h-full w-0.5 bg-white shadow-[0_0_0_1px_rgb(0_0_0/0.25)]"></div>
      <div
        class="compare-knob absolute top-1/2 left-1/2 flex size-9 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full bg-white text-stone-700 shadow"
      >
        <svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="m9 6-6 6 6 6m6-12 6 6-6 6" />
        </svg>
      </div>
    </div>
    <input
      type="range"
      min="0"
      max="100"
      step="1"
      bind:value={position}
      aria-label={t("compare.label")}
      aria-valuetext={`${t("compare.before")} ${position} %, ${t("compare.after")} ${100 - position} %`}
      class="absolute inset-0 size-full cursor-ew-resize opacity-0"
    />
    <span class="absolute top-3 left-3 rounded-md bg-black/60 px-2 py-0.5 text-xs text-white">
      {t("compare.before")}
    </span>
    <span class="absolute top-3 right-3 rounded-md bg-black/60 px-2 py-0.5 text-xs text-white">
      {t("compare.after")}
    </span>
  {/if}

  {#if working}
    <div
      class="absolute inset-x-0 bottom-0 flex items-center justify-center gap-2 bg-black/55 py-2 text-sm text-white"
      role="status"
    >
      <span class="size-3 animate-pulse rounded-full bg-accent-300"></span>
      {t("compare.working")}
    </div>
  {/if}
</div>

<style>
  .compare {
    --pos: 50%;
  }
  /* The result covers the right part, from the divider on. */
  .compare-after {
    clip-path: inset(0 0 0 var(--pos));
  }
  .compare-handle {
    left: var(--pos);
  }
  .compare:has(input:focus-visible) .compare-knob {
    outline: 2px solid var(--color-accent-600);
    outline-offset: 2px;
  }
</style>
