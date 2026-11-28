<script lang="ts">
  import { fromDataTransfer, fromInput, type NameFilter, type Picked } from "../files";
  import { t } from "../i18n/i18n.svelte";

  let {
    onfiles,
    filter,
    maxFiles,
    limitText = "",
  }: {
    onfiles: (picked: Picked) => void;
    filter: NameFilter;
    maxFiles: number;
    limitText?: string;
  } = $props();

  let active = $state(false);
  let fileInput: HTMLInputElement | undefined = $state();
  let folderInput: HTMLInputElement | undefined = $state();
  let depth = 0;

  async function drop(e: DragEvent) {
    e.preventDefault();
    active = false;
    depth = 0;
    if (!e.dataTransfer) {
      return;
    }
    const picked = await fromDataTransfer(e.dataTransfer, filter, maxFiles);
    if (picked.files.length > 0 || picked.removed.length > 0) {
      onfiles(picked);
    }
  }

  function picked(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const picked = fromInput(input.files, filter, maxFiles);
    input.value = "";
    if (picked.files.length > 0 || picked.removed.length > 0) {
      onfiles(picked);
    }
  }
</script>

<section
  aria-labelledby="drop-title"
  class={[
    "card flex flex-col items-center justify-center gap-4 border-2 border-dashed px-6 py-16 text-center transition-colors sm:py-24",
    active
      ? "border-accent-500 bg-accent-50 dark:bg-accent-950"
      : "border-stone-300 dark:border-stone-700",
  ]}
  ondragenter={(e) => {
    e.preventDefault();
    depth++;
    active = true;
  }}
  ondragover={(e) => e.preventDefault()}
  ondragleave={() => {
    depth = Math.max(0, depth - 1);
    active = depth > 0;
  }}
  ondrop={drop}
>
  <svg
    aria-hidden="true"
    viewBox="0 0 24 24"
    class="size-10 text-accent-700 dark:text-accent-300"
    fill="none"
    stroke="currentColor"
    stroke-width="1.5"
  >
    <path
      stroke-linecap="round"
      stroke-linejoin="round"
      d="M3 16.5v1.5A1.5 1.5 0 0 0 4.5 19.5h15a1.5 1.5 0 0 0 1.5-1.5v-1.5M12 3v12m0-12 4 4m-4-4-4 4"
    />
  </svg>
  <h2 id="drop-title" class="text-xl font-semibold">
    {active ? t("drop.active") : t("drop.title")}
  </h2>
  <p class="muted max-w-md text-sm">{t("drop.hint")}</p>
  <div class="flex flex-wrap justify-center gap-3">
    <button type="button" class="btn-primary" onclick={() => fileInput?.click()}>
      {t("drop.chooseFile")}
    </button>
    <button type="button" class="btn-secondary" onclick={() => folderInput?.click()}>
      {t("drop.chooseFolder")}
    </button>
  </div>
  {#if limitText}
    <p class="muted text-xs">{limitText}</p>
  {/if}
  <input
    bind:this={fileInput}
    type="file"
    multiple
    accept="image/*,.heic,.heif,.avif,.jxl,.zip,application/zip"
    class="hidden"
    onchange={picked}
  />
  <input bind:this={folderInput} type="file" webkitdirectory class="hidden" onchange={picked} />
</section>
