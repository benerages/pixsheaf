// Runs one image through the pipeline as a one-file job and keeps the
// result as a blob: the same bytes serve as preview and as download, so
// nothing is processed twice. A new run cancels the previous one.
import {
  ApiError,
  createJob,
  deleteJob,
  downloadResult,
  uploadFile,
  waitForJob,
  type JobFile,
  type JobHandle,
  type Pipeline,
} from "./api/client";

export type PreviewState = "idle" | "working" | "done" | "error";

export class Preview {
  state = $state<PreviewState>("idle");
  url = $state("");
  /**
   * The same bytes as a generic binary type for the download link: browsers
   * that can show an image type (Firefox) may open it instead of saving it,
   * even with the download attribute.
   */
  downloadUrl = $state("");
  blob = $state<Blob | null>(null);
  output = $state<JobFile["output"] | null>(null);
  input = $state<JobFile["input"] | null>(null);
  error = $state("");
  errorDetail = $state("");

  private controller: AbortController | null = null;
  private job: JobHandle | null = null;

  async run(file: File, name: string, pipeline: Pipeline): Promise<void> {
    this.cancel();
    const controller = new AbortController();
    this.controller = controller;
    const signal = controller.signal;
    this.state = "working";
    this.error = "";
    try {
      const job = await createJob(pipeline, 1, undefined, signal);
      this.job = job;
      await uploadFile(job, file, name, signal);
      const done = await waitForJob(job, () => {}, signal);
      const result = done.files[0];
      this.input = result?.input ?? null;
      if (!result || result.status !== "done" || !result.output) {
        this.fail(result?.error?.code ?? "unknown");
        await deleteJob(job);
        return;
      }
      const blob = await downloadResult(job, signal);
      this.job = null;
      if (signal.aborted) {
        return;
      }
      this.setResult(blob, result.output);
    } catch (err) {
      if (signal.aborted) {
        return;
      }
      if (err instanceof ApiError) {
        this.fail(err.code, err.problem?.errors?.map((e) => e.message).join("; ") ?? "");
      } else {
        this.fail("network");
      }
    }
  }

  private setResult(blob: Blob, output: NonNullable<JobFile["output"]>): void {
    this.clearUrl();
    this.blob = blob;
    this.output = output;
    this.url = URL.createObjectURL(blob);
    this.downloadUrl = URL.createObjectURL(new Blob([blob], { type: "application/octet-stream" }));
    this.state = "done";
  }

  private fail(code: string, detail = ""): void {
    this.state = "error";
    this.error = code;
    this.errorDetail = detail;
  }

  /** Stops a running preview and deletes its job on the server. */
  cancel(): void {
    this.controller?.abort();
    this.controller = null;
    if (this.job) {
      void deleteJob(this.job);
      this.job = null;
    }
  }

  private clearUrl(): void {
    for (const url of [this.url, this.downloadUrl]) {
      if (url) {
        URL.revokeObjectURL(url);
      }
    }
    this.url = "";
    this.downloadUrl = "";
    this.blob = null;
  }

  reset(): void {
    this.cancel();
    this.clearUrl();
    this.output = null;
    this.input = null;
    this.state = "idle";
    this.error = "";
  }
}
