<script lang="ts">
  import { onMount } from "svelte";
  import { getCapabilities, type Capabilities } from "./lib/api/client";
  import BatchView from "./lib/components/BatchView.svelte";
  import DropZone from "./lib/components/DropZone.svelte";
  import Header from "./lib/components/Header.svelte";
  import SingleView from "./lib/components/SingleView.svelte";
  import { Editor } from "./lib/editor.svelte";
  import { isZip, type PickedFile } from "./lib/files";
  import { formatBytes } from "./lib/format";
  import { i18n, numberLocale, t } from "./lib/i18n/i18n.svelte";
  import { presets } from "./lib/presets.svelte";

  const editor = new Editor();
  let caps = $state<Capabilities | null>(null);
  let files = $state<PickedFile[]>([]);
  let batchMode = $state(false);
  let main: HTMLElement | undefined = $state();

  onMount(() => {
    document.documentElement.lang = i18n.locale;
    void presets.load();
    getCapabilities()
      .then((c) => (caps = c))
      .catch(() => {});
    const paste = (e: ClipboardEvent) => {
      const list = [...(e.clipboardData?.files ?? [])];
      if (list.length > 0 && files.length === 0) {
        e.preventDefault();
        void choose(
          list.map((file) => ({ file, path: file.name || "pasted-image" })),
          false,
        );
      }
    };
    window.addEventListener("paste", paste);
    return () => window.removeEventListener("paste", paste);
  });

  // One image is the main workflow; several files, a folder, or an archive
  // switch to batch mode without any extra step.
  async function choose(picked: PickedFile[], folder: boolean) {
    const first = picked[0];
    const single = picked.length === 1 && first !== undefined && !folder && !(await isZip(first.file));
    files = picked;
    batchMode = !single;
    main?.focus();
  }

  function reset() {
    files = [];
    batchMode = false;
  }

  const maxFiles = $derived(caps?.limits.maxFilesPerJob ?? 50);
  const limitText = $derived(
    caps ? t("drop.limits", { size: formatBytes(caps.limits.maxUploadBytes, numberLocale()) }) : "",
  );
</script>

<a href="#main" class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:rounded-lg focus:bg-white focus:px-3 focus:py-2">
  {t("app.skip")}
</a>
<Header onhome={reset} />
<main id="main" tabindex="-1" bind:this={main} class="mx-auto max-w-6xl px-4 pb-16 sm:px-6">
  {#if files.length === 0}
    <div class="mx-auto max-w-3xl pt-6 sm:pt-12">
      <h1 class="mb-6 text-center text-2xl font-semibold tracking-tight sm:text-3xl">{t("app.tagline")}</h1>
      <DropZone onfiles={choose} {maxFiles} {limitText} />
    </div>
  {:else if batchMode}
    <BatchView bind:files {editor} onreset={reset} />
  {:else if files[0]}
    {#key files[0]}
      <SingleView picked={files[0]} {editor} onreset={reset} />
    {/key}
  {/if}
</main>
