<script lang="ts">
  import { i18n, locales, setLocale, t, type Locale } from "../i18n/i18n.svelte";

  let { onhome }: { onhome: () => void } = $props();
</script>

<header class="mx-auto flex w-full max-w-6xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
  <button type="button" class="flex items-center gap-2 rounded-lg" onclick={onhome}>
    <picture>
      <source srcset="/pixsheaf-mark-dark.svg" media="(prefers-color-scheme: dark)" />
      <img src="/pixsheaf-mark.svg" alt="" width="28" height="28" />
    </picture>
    <span class="text-lg font-semibold tracking-tight">pixsheaf</span>
  </button>
  <label class="flex items-center gap-2 text-sm">
    <span class="sr-only">{t("lang.label")}</span>
    <select
      class="field w-auto py-1"
      value={i18n.locale}
      onchange={(e) => setLocale((e.currentTarget as HTMLSelectElement).value as Locale)}
    >
      {#each locales as l (l.id)}
        <option value={l.id}>{l.name}</option>
      {/each}
    </select>
  </label>
</header>
