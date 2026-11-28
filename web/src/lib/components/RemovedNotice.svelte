<script lang="ts">
  import { t } from "../i18n/i18n.svelte";

  // Lists files that were left out because their format is not supported.
  let {
    names,
    formats = null,
    ondismiss,
  }: { names: string[]; formats?: readonly string[] | null; ondismiss: () => void } = $props();

  const shown = 10;
  const supported = $derived(
    formats
      ?.filter((f) => f !== "zip")
      .map((f) => f.toUpperCase())
      .concat("ZIP")
      .join(", ") ?? "",
  );
</script>

{#if names.length > 0}
  <div
    class="flex items-start gap-3 rounded-xl border border-amber-300 bg-amber-50 p-4 text-sm text-amber-950 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-50"
    role="status"
  >
    <div class="min-w-0 flex-1 space-y-2">
      <p class="font-medium">
        {names.length === 1 ? t("removed.titleOne") : t("removed.title", { count: names.length })}
      </p>
      <ul class="list-disc space-y-0.5 pl-5 break-all">
        {#each names.slice(0, shown) as name, i (i)}
          <li>{name}</li>
        {/each}
        {#if names.length > shown}
          <li class="list-none">{t("removed.more", { count: names.length - shown })}</li>
        {/if}
      </ul>
      {#if supported}
        <p>{t("removed.supported", { formats: supported })}</p>
      {/if}
    </div>
    <button type="button" class="btn-quiet shrink-0" onclick={ondismiss}>
      {t("removed.dismiss")}
    </button>
  </div>
{/if}
