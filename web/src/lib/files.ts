// Collecting files from drops, pickers, and the clipboard. Folder drops
// are walked recursively; paths are kept so that the server can rebuild
// the folder structure in the ZIP download.

export interface PickedFile {
  file: File;
  /** Relative path including folders, such as "Holiday/beach.jpg". */
  path: string;
}

/** The result of a selection: supported files and the names left out. */
export interface Picked {
  files: PickedFile[];
  /** Paths of files left out because their format is not supported. */
  removed: string[];
  folder: boolean;
}

/** File name extensions the server accepts per upload format (ADR 0023). */
const extensionsByFormat: Record<string, readonly string[]> = {
  jpeg: ["jpg", "jpeg", "jpe", "jfif"],
  png: ["png"],
  webp: ["webp"],
  avif: ["avif"],
  gif: ["gif"],
  tiff: ["tif", "tiff"],
  bmp: ["bmp"],
  jxl: ["jxl"],
  heif: ["heic", "heif", "hif"],
  zip: ["zip"],
};

/** Files that operating systems add to folders; left out without notice. */
const systemFiles = new Set(["thumbs.db", "desktop.ini"]);

/** At most this many removed names are kept for the notice. */
const maxRemovedNames = 1000;

export type NameCheck = "keep" | "remove" | "ignore";
export type NameFilter = (path: string) => NameCheck;

/**
 * Sorts a path by its extension: "keep" for the formats the server accepts
 * (all known formats until the capabilities are loaded), "ignore" for hidden
 * and system files such as .DS_Store or macOS "._" files, "remove" for the
 * rest. The server checks the extension and the content again.
 */
export function nameFilter(formats: readonly string[] | null): NameFilter {
  const allowed = new Set(
    (formats ?? Object.keys(extensionsByFormat)).flatMap((f) => extensionsByFormat[f] ?? []),
  );
  return (path) => {
    const base = path.slice(path.lastIndexOf("/") + 1);
    if (base.startsWith(".") || systemFiles.has(base.toLowerCase())) {
      return "ignore";
    }
    const dot = base.lastIndexOf(".");
    return dot > 0 && allowed.has(base.slice(dot + 1).toLowerCase()) ? "keep" : "remove";
  };
}

/** Collects supported files up to a limit and remembers the others. */
class Collector {
  files: PickedFile[] = [];
  removed: string[] = [];

  constructor(
    private readonly filter: NameFilter,
    private readonly max: number,
  ) {}

  get full(): boolean {
    return this.files.length >= this.max;
  }

  /** Sorts a path; true when the file is to be kept and there is room. */
  take(path: string): boolean {
    switch (this.filter(path)) {
      case "keep":
        return !this.full;
      case "remove":
        if (this.removed.length < maxRemovedNames) {
          this.removed.push(path);
        }
        return false;
      default:
        return false;
    }
  }
}

/** Sorts files that already have a path, such as pasted ones. */
export function sift(list: PickedFile[], filter: NameFilter, max: number): Picked {
  const c = new Collector(filter, max);
  for (const f of list) {
    if (c.take(f.path)) {
      c.files.push(f);
    }
  }
  return { files: c.files, removed: c.removed, folder: false };
}

function readEntries(reader: FileSystemDirectoryReader): Promise<FileSystemEntry[]> {
  return new Promise((resolve, reject) => reader.readEntries(resolve, reject));
}

function entryFile(entry: FileSystemFileEntry): Promise<File> {
  return new Promise((resolve, reject) => entry.file(resolve, reject));
}

async function walk(entry: FileSystemEntry, prefix: string, out: Collector): Promise<void> {
  if (out.full) {
    return;
  }
  if (entry.isFile) {
    // Only kept files are read.
    const path = prefix + entry.name;
    if (out.take(path)) {
      out.files.push({ file: await entryFile(entry as FileSystemFileEntry), path });
    }
    return;
  }
  if (entry.isDirectory) {
    const reader = (entry as FileSystemDirectoryEntry).createReader();
    // readEntries returns the entries in chunks until it returns none.
    for (;;) {
      const batch = await readEntries(reader);
      if (batch.length === 0) {
        break;
      }
      for (const child of batch) {
        await walk(child, `${prefix}${entry.name}/`, out);
      }
    }
  }
}

/**
 * Supported files of a drop, with folders walked. max bounds the number of
 * kept files; unsupported ones do not count against it.
 */
export async function fromDataTransfer(
  dt: DataTransfer,
  filter: NameFilter,
  max: number,
): Promise<Picked> {
  const out = new Collector(filter, max);
  let folder = false;
  const entries = [...dt.items]
    .filter((item) => item.kind === "file")
    .map((item) => item.webkitGetAsEntry())
    .filter((entry): entry is FileSystemEntry => entry !== null);
  if (entries.length > 0) {
    for (const entry of entries) {
      folder ||= entry.isDirectory;
      await walk(entry, "", out);
    }
    return { files: out.files, removed: out.removed, folder };
  }
  for (const file of dt.files) {
    if (out.take(file.name)) {
      out.files.push({ file, path: file.name });
    }
  }
  return { files: out.files, removed: out.removed, folder };
}

/** Supported files of an <input type="file">, with folder paths if it picked a folder. */
export function fromInput(list: FileList | null, filter: NameFilter, max: number): Picked {
  const all = [...(list ?? [])].map((file) => ({
    file,
    path: file.webkitRelativePath || file.name,
  }));
  return { ...sift(all, filter, max), folder: all.some((f) => f.path.includes("/")) };
}

/** A ZIP archive by its first bytes; the server checks again. */
export async function isZip(file: File): Promise<boolean> {
  const head = new Uint8Array(await file.slice(0, 4).arrayBuffer());
  return head[0] === 0x50 && head[1] === 0x4b && (head[2] === 0x03 || head[2] === 0x05);
}
