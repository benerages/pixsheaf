// Typed client for the Pixsheaf API (api/openapi.yaml). The UI uses this
// API exclusively. The job token is kept in memory only; the server also
// sets it as a cookie for the job's path, which plain links and
// EventSource use for downloads and events (ADR 0019).
import type { components } from "./schema";

export type Schemas = components["schemas"];
export type Pipeline = Schemas["Pipeline"];
export type Job = Schemas["Job"];
export type JobFile = Schemas["JobFile"];
export type Capabilities = Schemas["Capabilities"];
export type Preset = Schemas["Preset"];
export type Naming = Schemas["Naming"];
export type Problem = Schemas["Problem"];

const base = "/api/v1";

/** An error response from the API (RFC 9457). */
export class ApiError extends Error {
  readonly status: number;
  readonly problem: Problem | null;
  readonly retryAfter: number | null;

  constructor(status: number, problem: Problem | null, retryAfter: number | null) {
    super(problem?.detail ?? problem?.title ?? `HTTP ${status}`);
    this.status = status;
    this.problem = problem;
    this.retryAfter = retryAfter;
  }

  get code(): string {
    return this.problem?.code ?? "unknown";
  }
}

async function check(res: Response): Promise<Response> {
  if (res.ok) {
    return res;
  }
  let problem: Problem | null = null;
  try {
    problem = (await res.json()) as Problem;
  } catch {
    // Not a problem document.
  }
  const retry = Number(res.headers.get("Retry-After"));
  throw new ApiError(res.status, problem, Number.isFinite(retry) && retry > 0 ? retry : null);
}

export async function getCapabilities(): Promise<Capabilities> {
  return (await (await check(await fetch(`${base}/capabilities`))).json()) as Capabilities;
}

export async function getPresets(): Promise<Preset[]> {
  const res = await check(await fetch(`${base}/presets`));
  const body = (await res.json()) as { presets: Preset[] };
  return body.presets;
}

/** A job with its token, as returned by createJob. */
export interface JobHandle {
  id: string;
  token: string;
}

export async function createJob(
  pipeline: Pipeline,
  expectedFiles: number,
  naming?: Naming,
  signal?: AbortSignal,
): Promise<JobHandle> {
  const res = await check(
    await fetch(`${base}/jobs`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pipeline, expectedFiles, naming }),
      signal,
    }),
  );
  const body = (await res.json()) as Schemas["JobCreated"];
  return { id: body.job.id, token: body.token };
}

function auth(job: JobHandle): HeadersInit {
  return { Authorization: `Bearer ${job.token}` };
}

export async function getJob(job: JobHandle, signal?: AbortSignal): Promise<Job> {
  const res = await check(await fetch(`${base}/jobs/${job.id}`, { headers: auth(job), signal }));
  return (await res.json()) as Job;
}

export async function deleteJob(job: JobHandle): Promise<void> {
  try {
    await fetch(`${base}/jobs/${job.id}`, { method: "DELETE", headers: auth(job) });
  } catch {
    // Best effort: the job expires on the server anyway.
  }
}

export async function cancelFile(job: JobHandle, fileId: string): Promise<void> {
  await check(
    await fetch(`${base}/jobs/${job.id}/files/${fileId}`, { method: "DELETE", headers: auth(job) }),
  );
}

/**
 * Uploads one file. name may contain a folder path (folder uploads). The
 * upload is retried while the server answers 503 (queue or storage full).
 */
export async function uploadFile(
  job: JobHandle,
  file: Blob,
  name: string,
  signal?: AbortSignal,
): Promise<JobFile> {
  for (;;) {
    const form = new FormData();
    form.append("file", file, name);
    const res = await fetch(`${base}/jobs/${job.id}/files`, {
      method: "POST",
      headers: auth(job),
      body: form,
      signal,
    });
    if (res.status === 503) {
      const wait = Number(res.headers.get("Retry-After")) || 2;
      await sleep(wait * 1000, signal);
      continue;
    }
    return (await (await check(res)).json()) as JobFile;
  }
}

/** Waits until the job is done or canceled, reporting every change. */
export async function waitForJob(
  job: JobHandle,
  onChange: (job: Job) => void,
  signal?: AbortSignal,
): Promise<Job> {
  // Short polling is simple and robust for one file; batches use events.
  let delay = 150;
  for (;;) {
    const state = await getJob(job, signal);
    onChange(state);
    if (state.status === "done" || state.status === "canceled") {
      return state;
    }
    await sleep(delay, signal);
    delay = Math.min(delay * 1.5, 1000);
  }
}

/**
 * Follows a job with Server-Sent Events (the job cookie authorizes the
 * request). Returns a function that closes the stream.
 */
export function followJob(
  job: JobHandle,
  onJob: (job: Job) => void,
  onFile: (file: JobFile) => void,
): () => void {
  const source = new EventSource(`${base}/jobs/${job.id}/events`);
  source.addEventListener("job", (e) => {
    const state = JSON.parse((e as MessageEvent<string>).data) as Job;
    onJob(state);
    if (state.status === "done" || state.status === "canceled") {
      source.close();
    }
  });
  source.addEventListener("file", (e) => {
    onFile(JSON.parse((e as MessageEvent<string>).data) as JobFile);
  });
  return () => source.close();
}

/** Downloads the result of a one-file job as a blob (one time). */
export async function downloadResult(job: JobHandle, signal?: AbortSignal): Promise<Blob> {
  const res = await check(
    await fetch(`${base}/jobs/${job.id}/download`, { headers: auth(job), signal }),
  );
  return res.blob();
}

/** URL of a one-time download that the job cookie authorizes. */
export function downloadUrl(job: JobHandle, fileId?: string): string {
  return fileId
    ? `${base}/jobs/${job.id}/files/${fileId}/download`
    : `${base}/jobs/${job.id}/download`;
}

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal?.addEventListener(
      "abort",
      () => {
        clearTimeout(timer);
        reject(signal.reason);
      },
      { once: true },
    );
  });
}
