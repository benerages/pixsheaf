<script lang="ts">
  import { onMount } from "svelte";
  import { getCapabilities, type Capabilities } from "./lib/api/client";
  import BatchView from "./lib/components/BatchView.svelte";
  import DropZone from "./lib/components/DropZone.svelte";
  import Footer from "./lib/components/Footer.svelte";
  import Header from "./lib/components/Header.svelte";
  import RemovedNotice from "./lib/components/RemovedNotice.svelte";
  import SingleView from "./lib/components/SingleView.svelte";
  import { Editor } from "./lib/editor.svelte";
  import { isZip, nameFilter, sift, type Picked } from "./lib/files";
  import { formatBytes } from "./lib/format";
  import { i18n, numberLocale, t } from "./lib/i18n/i18n.svelte";
  import { presets } from "./lib/presets.svelte";

  const editor = new Editor();
  let caps = $state<Capabilities | null>(null);
  let files = $state<Picked["files"]>([]);
  /** Files of the last selection left out because of their format. */
  let removed = $state<string[]>([]);
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
          sift(
            list.map((file) => ({ file, path: pastedName(file) })),
            filter,
            maxFiles,
          ),
        );
      }
    };
    window.addEventListener("paste", paste);
    return () => window.removeEventListener("paste", paste);
  });

  // Pasted images may have no name; the API needs an image extension.
  function pastedName(file: File): string {
    if (file.name.includes(".")) {
      return file.name;
    }
    const sub = file.type.split("/")[1] ?? "png";
    return `pasted-image.${sub === "jpeg" ? "jpg" : sub}`;
  }

  // One image is the main workflow; several files, a folder, or an archive
  // switch to batch mode without any extra step. Unsupported files are left
  // out and listed in a notice; if nothing is left, the drop zone stays.
  async function choose({ files: picked, removed: skipped, folder }: Picked) {
    removed = skipped;
    const first = picked[0];
    if (first === undefined) {
      return;
    }
    const single = picked.length === 1 && !folder && !(await isZip(first.file));
    files = picked;
    batchMode = !single;
    main?.focus();
  }

  function reset() {
    files = [];
    removed = [];
    batchMode = false;
  }

  const maxFiles = $derived(caps?.limits.maxFilesPerJob ?? 50);
  const formats = $derived(caps?.inputFormats ?? null);
  const filter = $derived(nameFilter(formats));
  const limitText = $derived(
    caps ? t("drop.limits", { size: formatBytes(caps.limits.maxUploadBytes, numberLocale()) }) : "",
  );
</script>

<a
  href="#main"
  class="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:rounded-lg focus:bg-white focus:px-3 focus:py-2"
>
  {t("app.skip")}
</a>
<Header onhome={reset} />
<main
  id="main"
  tabindex="-1"
  bind:this={main}
  class="mx-auto w-full max-w-6xl flex-1 px-4 pb-16 sm:px-6"
>
  {#if removed.length > 0}
    <div class={["mb-4 pt-4", files.length === 0 && "mx-auto max-w-3xl"]}>
      <RemovedNotice names={removed} {formats} ondismiss={() => (removed = [])} />
    </div>
  {/if}
  {#if files.length === 0}
    <div class="mx-auto max-w-3xl pt-6 sm:pt-12">
      <h1 class="mb-6 text-center text-2xl font-semibold tracking-tight sm:text-3xl">
        {t("app.tagline")}
      </h1>
      <DropZone onfiles={choose} {filter} {maxFiles} {limitText} />
    </div>
  {:else if batchMode}
    <BatchView bind:files {editor} {formats} onreset={reset} />
  {:else if files[0]}
    {#key files[0]}
      <SingleView picked={files[0]} {editor} onreset={reset} />
    {/key}
  {/if}
</main>
<Footer version={caps?.version ?? null} />
