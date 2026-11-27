// Batch mode: one job for all selected files. Uploads run a few at a time;
// progress comes from Server-Sent Events. Archives are unpacked on the
// server, and their images appear as new files of the job.
import {
  ApiError,
  cancelFile,
  createJob,
  deleteJob,
  downloadUrl,
  followJob,
  uploadFile,
  type Job,
  type JobFile,
  type JobHandle,
  type Naming,
  type Pipeline,
} from "./api/client";
import type { PickedFile } from "./files";

const parallelUploads = 3;

export interface Row {
  key: string;
  name: string;
  /** Local file before upload; null for files from an archive. */
  local: PickedFile | null;
  bytes: number;
  status: "waiting" | "uploading" | JobFile["status"];
  file: JobFile | null;
  /** Set when the result was downloaded on its own. */
  downloaded: boolean;
  error: string;
}

export class Batch {
  rows = $state<Row[]>([]);
  job = $state<JobHandle | null>(null);
  jobStatus = $state<Job["status"] | "">("");
  zipDownloaded = $state(false);
  error = $state("");

  private stop: (() => void) | null = null;
  private controller: AbortController | null = null;

  setFiles(files: PickedFile[]): void {
    this.reset();
    this.rows = files.map((f, i) => ({
      key: `local-${i}`,
      name: f.path,
      local: f,
      bytes: f.file.size,
      status: "waiting",
      file: null,
      downloaded: false,
      error: "",
    }));
  }

  get started(): boolean {
    return this.job !== null;
  }

  /** Files that are images (archives are containers, not results). */
  get images(): Row[] {
    return this.rows.filter((r) =>
      r.file ? r.file.kind !== "archive" : !r.local?.path.toLowerCase().endsWith(".zip"),
    );
  }

  get finished(): number {
    return this.images.filter((r) => ["done", "failed", "canceled"].includes(r.status)).length;
  }

  get hasArchives(): boolean {
    return this.rows.some(
      (r) => r.file?.kind === "archive" || r.local?.path.toLowerCase().endsWith(".zip"),
    );
  }

  async start(pipeline: Pipeline, naming: Naming): Promise<void> {
    const local = this.rows.filter((r) => r.local);
    if (local.length === 0) {
      return;
    }
    this.error = "";
    const controller = new AbortController();
    this.controller = controller;
    try {
      const job = await createJob(pipeline, local.length, naming, controller.signal);
      this.job = job;
      this.stop = followJob(
        job,
        (state) => this.onJob(state),
        (file) => this.onFile(file),
      );
      let next = 0;
      const worker = async () => {
        while (next < local.length) {
          const row = local[next++];
          if (!row?.local) {
            continue;
          }
          this.update(row.key, { status: "uploading" });
          try {
            const file = await uploadFile(job, row.local.file, row.local.path, controller.signal);
            this.update(row.key, { key: file.id, file, status: file.status });
          } catch (err) {
            if (controller.signal.aborted) {
              return;
            }
            this.update(row.key, {
              status: "failed",
              error: err instanceof ApiError ? err.code : "network",
            });
          }
        }
      };
      await Promise.all(Array.from({ length: parallelUploads }, worker));
    } catch (err) {
      if (!controller.signal.aborted) {
        this.error = err instanceof ApiError ? err.code : "network";
      }
    }
  }

  private update(key: string, patch: Partial<Row>): void {
    this.rows = this.rows.map((r) => (r.key === key ? { ...r, ...patch } : r));
  }

  private onJob(state: Job): void {
    this.jobStatus = state.status;
    for (const file of state.files) {
      this.onFile(file);
    }
  }

  private onFile(file: JobFile): void {
    const existing = this.rows.find((r) => r.key === file.id);
    if (existing) {
      this.update(file.id, { file, status: file.status, error: file.error?.code ?? "" });
      return;
    }
    if (file.archiveId) {
      // An image unpacked from an archive.
      this.rows = [
        ...this.rows,
        {
          key: file.id,
          name: file.source ?? "",
          local: null,
          bytes: file.input?.bytes ?? 0,
          status: file.status,
          file,
          downloaded: false,
          error: file.error?.code ?? "",
        },
      ];
    }
  }

  archiveName(file: JobFile | null): string {
    if (!file?.archiveId) {
      return "";
    }
    return this.rows.find((r) => r.key === file.archiveId)?.name ?? "";
  }

  fileUrl(row: Row): string {
    return this.job && row.file ? downloadUrl(this.job, row.file.id) : "";
  }

  zipUrl(): string {
    return this.job ? downloadUrl(this.job) : "";
  }

  markDownloaded(key: string): void {
    this.update(key, { downloaded: true });
  }

  async cancel(row: Row): Promise<void> {
    if (this.job && row.file) {
      try {
        await cancelFile(this.job, row.file.id);
        this.update(row.key, { status: "canceled" });
      } catch {
        // The file may have finished in the meantime.
      }
    }
  }

  async cancelAll(): Promise<void> {
    this.controller?.abort();
    if (this.job) {
      await deleteJob(this.job);
    }
    this.rows = this.rows.map((r) =>
      ["done", "failed"].includes(r.status) ? r : { ...r, status: "canceled" },
    );
    this.jobStatus = "canceled";
    this.stop?.();
  }

  reset(): void {
    this.controller?.abort();
    this.stop?.();
    if (this.job && !this.zipDownloaded) {
      void deleteJob(this.job);
    }
    this.job = null;
    this.jobStatus = "";
    this.zipDownloaded = false;
    this.rows = [];
    this.error = "";
  }
}
