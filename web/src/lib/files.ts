// Collecting files from drops, pickers, and the clipboard. Folder drops
// are walked recursively; paths are kept so that the server can rebuild
// the folder structure in the ZIP download.

export interface PickedFile {
  file: File;
  /** Relative path including folders, such as "Holiday/beach.jpg". */
  path: string;
}

function readEntries(reader: FileSystemDirectoryReader): Promise<FileSystemEntry[]> {
  return new Promise((resolve, reject) => reader.readEntries(resolve, reject));
}

function entryFile(entry: FileSystemFileEntry): Promise<File> {
  return new Promise((resolve, reject) => entry.file(resolve, reject));
}

async function walk(
  entry: FileSystemEntry,
  prefix: string,
  out: PickedFile[],
  max: number,
): Promise<void> {
  if (out.length >= max) {
    return;
  }
  if (entry.isFile) {
    const file = await entryFile(entry as FileSystemFileEntry);
    out.push({ file, path: prefix + file.name });
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
        await walk(child, `${prefix}${entry.name}/`, out, max);
      }
    }
  }
}

/** Files of a drop, with folders walked. max bounds the number of files. */
export async function fromDataTransfer(
  dt: DataTransfer,
  max: number,
): Promise<{ files: PickedFile[]; folder: boolean }> {
  const out: PickedFile[] = [];
  let folder = false;
  const entries = [...dt.items]
    .filter((item) => item.kind === "file")
    .map((item) => item.webkitGetAsEntry())
    .filter((entry): entry is FileSystemEntry => entry !== null);
  if (entries.length > 0) {
    for (const entry of entries) {
      folder ||= entry.isDirectory;
      await walk(entry, "", out, max);
    }
    return { files: out, folder };
  }
  for (const file of dt.files) {
    out.push({ file, path: file.name });
  }
  return { files: out.slice(0, max), folder };
}

/** Files of an <input type="file">, with folder paths if it picked a folder. */
export function fromInput(
  list: FileList | null,
  max: number,
): { files: PickedFile[]; folder: boolean } {
  const files = [...(list ?? [])].slice(0, max).map((file) => ({
    file,
    path: file.webkitRelativePath || file.name,
  }));
  return { files, folder: files.some((f) => f.path.includes("/")) };
}

/** A ZIP archive by its first bytes; the server checks again. */
export async function isZip(file: File): Promise<boolean> {
  const head = new Uint8Array(await file.slice(0, 4).arrayBuffer());
  return head[0] === 0x50 && head[1] === 0x4b && (head[2] === 0x03 || head[2] === 0x05);
}
