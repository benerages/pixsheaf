# API reference

<!-- Generated from api/openapi.yaml by `pnpm gen:docs` (scripts/generate-api.sh). Do not edit by hand; CI fails when it is out of date. -->

**Pixsheaf API** 1.0.0, OpenAPI 3.1.0. Base path: `/api/v1`.

The Pixsheaf API processes images with a declarative pipeline. The web UI
uses this API exclusively.

## Glossary

- **Job:** one request to process a set of one or more files with one
  pipeline. A job with one file returns the processed image directly; a
  job with several files returns a ZIP archive.
- **File (task):** one uploaded image inside a job. Each file is processed
  on its own, in an isolated worker process.
- **Pipeline:** the ordered list of steps applied to every file
  (ADR 0009).

## Flow

1. `POST /jobs` with a pipeline and the number of files to expect. The
   response contains the job and a secret job token.
2. `POST /jobs/{jobId}/files` once per file (uploads may run in
   parallel). Processing of each file starts as soon as it is uploaded.
   The job accepts no further uploads once `expectedFiles` have arrived.
   A ZIP archive counts as one upload; it is unpacked in the worker
   sandbox and each image in it becomes a file of the job (ADR 0022).
3. Follow progress with `GET /jobs/{jobId}/events` (Server-Sent Events)
   or poll `GET /jobs/{jobId}`.
4. `GET /jobs/{jobId}/download` once the job is `done`. The result can be
   downloaded once; afterwards the job and all its data are deleted.
   Jobs that are not downloaded expire at `expiresAt`.

## Authentication and CSRF (ADR 0019)

No account is needed (ADR 0015). Creating a job returns a secret **job
token**. Every request for that job must present it, either:

- as `Authorization: Bearer <token>` (required for all requests that
  change state), or
- as the `__Host-pixsheaf-{jobId}` cookie, which the server sets on job
  creation (`Secure; HttpOnly; SameSite=Strict; Path=/`). The job ID in
  the name binds the cookie to its job, so a browser can hold several
  jobs at once; the `__Host-` prefix keeps other hosts from setting it.
  The cookie is accepted only for `GET /jobs/{jobId}/events`,
  `GET /jobs/{jobId}/download`, and
  `GET /jobs/{jobId}/files/{fileId}/download`, so that browsers can use
  `EventSource` and plain download links.

The job ID alone grants nothing. State-changing requests are also
rejected when `Sec-Fetch-Site` is `cross-site` or `same-site`, or when
`Origin` does not match the server's origin. CORS is not enabled.

## Limits and fairness

Limits are listed by `GET /capabilities`. Each client (identified by its
IP address, taken from `X-Forwarded-For` only behind a configured trusted
proxy) may have a limited number of open jobs and requests per minute;
beyond that the server answers `429` with `Retry-After`. Files of
different clients are processed in turn, so one large batch does not
block others. When the queue or the storage is full, uploads get `503`
with `Retry-After`.

## Errors

Errors use `application/problem+json` (RFC 9457) with a stable `code`.

## Endpoints

| Method | Path | Summary | Access |
|---|---|---|---|
| GET | [`/capabilities`](#getcapabilities) | Formats and limits of this server | public |
| GET | [`/presets`](#listpresets) | Bundled presets | public |
| POST | [`/jobs`](#createjob) | Create a job | public |
| GET | [`/jobs/{jobId}`](#getjob) | Get a job and the status of its files | jobToken |
| DELETE | [`/jobs/{jobId}`](#deletejob) | Cancel a job and delete all its data | jobToken |
| POST | [`/jobs/{jobId}/files`](#uploadfile) | Upload one file | jobToken |
| GET | [`/jobs/{jobId}/files/{fileId}`](#getfile) | Get the status of one file | jobToken |
| DELETE | [`/jobs/{jobId}/files/{fileId}`](#cancelfile) | Cancel one file | jobToken |
| GET | [`/jobs/{jobId}/files/{fileId}/download`](#downloadfile) | Download one processed file once | jobToken or jobCookie |
| GET | [`/jobs/{jobId}/events`](#streamjobevents) | Follow job progress (Server-Sent Events) | jobToken or jobCookie |
| GET | [`/jobs/{jobId}/download`](#downloadjob) | Download the result once | jobToken or jobCookie |

### getCapabilities

`GET /capabilities`: Formats and limits of this server

Returns the input and output formats and the limits configured by the operator. Clients use it to validate input before uploading.

| Status | Response |
|---|---|
| 200 | Capabilities. [`Capabilities`](#capabilities) |
| 429 | A rate limit was hit. [`Problem`](#problem) |

### listPresets

`GET /presets`: Bundled presets

Returns the presets that ship with the server. A preset is a pipeline (and optionally a naming pattern) that a client sends unchanged in `POST /jobs`; the server does not store presets. Custom presets are kept by the client and exchanged as JSON in the same `Preset` format, so the same preset produces the same result in the UI and through the API.

| Status | Response |
|---|---|
| 200 | Bundled presets. `object` |
| 429 | A rate limit was hit. [`Problem`](#problem) |

### createJob

`POST /jobs`: Create a job

Creates a job with a validated pipeline. The response body contains the job token; the server also sets it as the `__Host-pixsheaf-{jobId}` cookie.

Request body (`application/json`): [`JobCreate`](#jobcreate)

| Status | Response |
|---|---|
| 201 | Job created. [`JobCreated`](#jobcreated) |
| 400 | The request is malformed. [`Problem`](#problem) |
| 403 | Rejected by the same-origin check (CSRF protection). [`Problem`](#problem) |
| 413 | A size limit was exceeded. [`Problem`](#problem) |
| 415 | The request body or the uploaded file has a type that is not allowed (`unsupported_media_type`, `unsupported_format`; ADR 0014, ADR 0023), or the operator disabled the file's format (`format_disabled`, ADR 0016). [`Problem`](#problem) |
| 422 | The pipeline is syntactically valid JSON but not a valid pipeline. [`Problem`](#problem) |
| 429 | A rate limit was hit. [`Problem`](#problem) |
| 503 | The processing queue is full. [`Problem`](#problem) |

### getJob

`GET /jobs/{jobId}`: Get a job and the status of its files

| Parameter | In | Required | Description |
|---|---|---|---|
| `jobId` | path | yes |  |

| Status | Response |
|---|---|
| 200 | The job. [`Job`](#job) |
| 401 | The job token is missing or wrong. [`Problem`](#problem) |
| 404 | The job or file does not exist, has expired, or was already downloaded. [`Problem`](#problem) |
| 429 | A rate limit was hit. [`Problem`](#problem) |

### deleteJob

`DELETE /jobs/{jobId}`: Cancel a job and delete all its data

| Parameter | In | Required | Description |
|---|---|---|---|
| `jobId` | path | yes |  |

| Status | Response |
|---|---|
| 204 | Job canceled and deleted. |
| 401 | The job token is missing or wrong. [`Problem`](#problem) |
| 403 | Rejected by the same-origin check (CSRF protection). [`Problem`](#problem) |
| 404 | The job or file does not exist, has expired, or was already downloaded. [`Problem`](#problem) |
| 429 | A rate limit was hit. [`Problem`](#problem) |

### uploadFile

`POST /jobs/{jobId}/files`: Upload one file

Uploads one image or one ZIP archive as a multipart form with a single part named `file`. The type is detected from the content (magic bytes); the part's `Content-Type` and file name are not used for that decision (ADR 0014). The file name must still end in an image extension (`.jpg`, `.jpeg`, `.jpe`, `.jfif`, `.png`, `.webp`, `.avif`, `.gif`, `.tif`, `.tiff`, `.bmp`, `.jxl`, `.heic`, `.heif`, `.hif`) or in `.zip`, compared case-insensitively; otherwise the upload gets 415 `unsupported_format` (ADR 0023). An extension that does not match the content is accepted, because the content decides and every output is a new encode. Formats the operator disabled (`PIXSHEAF_DISABLE_FORMATS`, ADR 0016) get 415 `format_disabled` and are missing from `GET /capabilities`. The file name may contain a relative folder path (folder uploads); it is kept in memory only, to build the output name and the folder structure of the ZIP download, and it is never logged. Processing starts immediately.
A ZIP archive counts as one upload and becomes a file of kind `archive`. It is unpacked in the worker sandbox (ADR 0022) within `maxArchiveBytes` and `maxArchiveEntries` and the job's file and size limits; each image entry then becomes a file of kind `image` with `archiveId` set. Directories are skipped; symbolic links, encrypted entries, nested archives, entries without an image extension or in a disabled format, and entries that are too large or compressed too strongly become failed files with a reason, so they appear in the report.

| Parameter | In | Required | Description |
|---|---|---|---|
| `jobId` | path | yes |  |

Request body (`multipart/form-data`): [`FileUpload`](#fileupload)

| Status | Response |
|---|---|
| 201 | File accepted and queued. [`JobFile`](#jobfile) |
| 400 | The request is malformed. [`Problem`](#problem) |
| 401 | The job token is missing or wrong. [`Problem`](#problem) |
| 403 | Rejected by the same-origin check (CSRF protection). [`Problem`](#problem) |
| 404 | The job or file does not exist, has expired, or was already downloaded. [`Problem`](#problem) |
| 409 | The job is not in a state that allows this request (for example, all expected files were already uploaded, or the job is not done yet). [`Problem`](#problem) |
| 413 | A size limit was exceeded. [`Problem`](#problem) |
| 415 | The request body or the uploaded file has a type that is not allowed (`unsupported_media_type`, `unsupported_format`; ADR 0014, ADR 0023), or the operator disabled the file's format (`format_disabled`, ADR 0016). [`Problem`](#problem) |
| 429 | A rate limit was hit. [`Problem`](#problem) |
| 503 | The processing queue is full. [`Problem`](#problem) |

### getFile

`GET /jobs/{jobId}/files/{fileId}`: Get the status of one file

| Parameter | In | Required | Description |
|---|---|---|---|
| `jobId` | path | yes |  |
| `fileId` | path | yes |  |

| Status | Response |
|---|---|
| 200 | The file. [`JobFile`](#jobfile) |
| 401 | The job token is missing or wrong. [`Problem`](#problem) |
| 404 | The job or file does not exist, has expired, or was already downloaded. [`Problem`](#problem) |
| 429 | A rate limit was hit. [`Problem`](#problem) |

### cancelFile

`DELETE /jobs/{jobId}/files/{fileId}`: Cancel one file

Cancels processing of one file and deletes its data. The file stays in the job with status `canceled`; the rest of the job continues.

| Parameter | In | Required | Description |
|---|---|---|---|
| `jobId` | path | yes |  |
| `fileId` | path | yes |  |

| Status | Response |
|---|---|
| 204 | File canceled. |
| 401 | The job token is missing or wrong. [`Problem`](#problem) |
| 403 | Rejected by the same-origin check (CSRF protection). [`Problem`](#problem) |
| 404 | The job or file does not exist, has expired, or was already downloaded. [`Problem`](#problem) |
| 429 | A rate limit was hit. [`Problem`](#problem) |

### downloadFile

`GET /jobs/{jobId}/files/{fileId}/download`: Download one processed file once

Returns the result of one file of a job, for example to save single files of a batch. Available once the file is `done`. The result can be downloaded once; afterwards it is deleted, the file shows `downloaded: true`, and the job's ZIP download leaves it out (the report still lists it). The job cookie is accepted here.

| Parameter | In | Required | Description |
|---|---|---|---|
| `jobId` | path | yes |  |
| `fileId` | path | yes |  |

| Status | Response |
|---|---|
| 200 | The processed image. [`Binary`](#binary) |
| 401 | The job token is missing or wrong. [`Problem`](#problem) |
| 404 | The job or file does not exist, has expired, or was already downloaded. [`Problem`](#problem) |
| 409 | The job is not in a state that allows this request (for example, all expected files were already uploaded, or the job is not done yet). [`Problem`](#problem) |
| 429 | A rate limit was hit. [`Problem`](#problem) |

### streamJobEvents

`GET /jobs/{jobId}/events`: Follow job progress (Server-Sent Events)

Streams progress as Server-Sent Events. Each event's `data` is one
JSON object:

- `event: job` with a `Job` whenever the job status changes,
- `event: file` with a `JobFile` whenever a file's status changes.

The first events describe the current state, so a client that
reconnects needs no other request. The stream ends after the job
reaches `done` or `canceled`. Comment lines (`: ping`) keep the
connection open. The job cookie is accepted here.

| Parameter | In | Required | Description |
|---|---|---|---|
| `jobId` | path | yes |  |

| Status | Response |
|---|---|
| 200 | Event stream. `string` |
| 401 | The job token is missing or wrong. [`Problem`](#problem) |
| 404 | The job or file does not exist, has expired, or was already downloaded. [`Problem`](#problem) |
| 429 | A rate limit was hit. [`Problem`](#problem) |

### downloadJob

`GET /jobs/{jobId}/download`: Download the result once

Returns the processed image when the job was created with `expectedFiles: 1` and that upload is an image; otherwise a ZIP archive, streamed without buffering, with every successfully processed file and a `report.json` (schema `Report`). Names inside the archive follow the job's `naming`; duplicates get a suffix such as ` (2)`. Available once the job is `done`. After the download the job and its data are deleted; a second request returns 404. Original bytes are never returned; every output is a new encode (ADR 0014). The job cookie is accepted here.

| Parameter | In | Required | Description |
|---|---|---|---|
| `jobId` | path | yes |  |

| Status | Response |
|---|---|
| 200 | The result. [`Binary`](#binary) |
| 401 | The job token is missing or wrong. [`Problem`](#problem) |
| 404 | The job or file does not exist, has expired, or was already downloaded. [`Problem`](#problem) |
| 409 | The job is not in a state that allows this request (for example, all expected files were already uploaded, or the job is not done yet). [`Problem`](#problem) |
| 429 | A rate limit was hit. [`Problem`](#problem) |

## Schemas

### Id

Random identifier with 128 bits of entropy (base64url, no padding).

Type: `string`.

### Token

Secret job token with 256 bits of entropy (base64url, no padding).

Type: `string`.

### Binary

Type: `string`.

### InputFormat

Formats accepted for upload, detected by magic bytes (ADR 0014). `bmp` must be uncompressed (1, 2, 4, 8, 24 or 32 bits per pixel; RLE is rejected as `unsupported_format`) and is limited to 32 megapixels. `zip` is accepted as an upload container only (ADR 0022).

One of: `jpeg`, `png`, `webp`, `gif`, `tiff`, `bmp`, `avif`, `heif`, `jxl`, `zip`.

### OutputFormat

Formats that can be produced. `pdf` embeds the image as a JPEG in a one-page A4 PDF (orientation follows the image, 0.5 inch margins, at most 96 DPI so small images are not enlarged). `bmp` is written as uncompressed 24-bit BMP; transparency is flattened onto white, as for `jpeg` and `pdf`. Animated GIF and WebP input stays animated when the output is `gif` or `webp` (every step runs on every frame; frame delays and the loop count are kept, and content-aware crop gravities fall back to `center` so that the frames stay aligned). Other output formats get the first frame. Animations with more frames than the limit fail with `too_many_frames`.

One of: `jpeg`, `png`, `webp`, `gif`, `tiff`, `bmp`, `avif`, `jxl`, `pdf`.

### Capabilities

| Property | Type | Required | Description |
|---|---|---|---|
| `pipelineVersion` | `integer` | yes | Highest pipeline version this server accepts. |
| `inputFormats` | array of [`InputFormat`](#inputformat) | yes |  |
| `outputFormats` | array of [`OutputFormat`](#outputformat) | yes |  |
| `limits` | [`Limits`](#limits) | yes |  |

### Limits

Limits configured by the operator; always at or below the schema maximums.

| Property | Type | Required | Description |
|---|---|---|---|
| `maxUploadBytes` | `integer` | yes |  |
| `maxFilesPerJob` | `integer` | yes |  |
| `maxJobBytes` | `integer` | yes |  |
| `maxInputPixels` | `integer` | yes |  |
| `maxOutputPixels` | `integer` | yes |  |
| `maxSteps` | `integer` | yes |  |
| `maxWatermarkBytes` | `integer` | yes | 0 means that image watermarks are disabled. |
| `maxFrames` | `integer` | yes | Frames of an animated GIF or WebP. |
| `maxArchiveBytes` | `integer` | yes | Size of one uploaded ZIP archive. |
| `maxArchiveEntries` | `integer` | yes | Entries (files and folders) in one ZIP archive. |
| `maxJobsPerClient` | `integer` | yes | Jobs one client may have open at the same time. |
| `requestsPerMinute` | `integer` | yes | API requests per client and minute (with a burst of the same size). |

### JobCreate

| Property | Type | Required | Description |
|---|---|---|---|
| `pipeline` | [`Pipeline`](#pipeline) | yes |  |
| `expectedFiles` | `integer` | yes | Number of files that will be uploaded. The job stops accepting uploads after this many files. With 1, the download is the image itself; with more, it is a ZIP archive. |
| `naming` | [`Naming`](#naming) | no |  |

### Naming

How output files are named. The pattern builds the name without its extension; the extension of the output format is always appended. Placeholders: `{name}` (uploaded name without folder and extension), `{index}` (upload position starting at 1, zero-padded to the number of files), `{width}`, `{height}` (output size), `{date}` (job creation date, UTC, `YYYY-MM-DD`), `{ext}` (output extension). Any other text is kept literally, except characters that are not allowed in names (they become `_`); `{` and `}` outside a known placeholder are rejected. Duplicate names get a suffix such as ` (2)`.

| Property | Type | Required | Description |
|---|---|---|---|
| `pattern` | `string` | no |  |
| `keepFolders` | `boolean` | no | Keep the folder structure of folder uploads and ZIP archives in the ZIP download. |

### Preset

A named pipeline for import, export, and the bundled presets. The same document is accepted by `POST /jobs` (as `pipeline` and `naming`).

| Property | Type | Required | Description |
|---|---|---|---|
| `format` | `string` | yes | Marks the document as a Pixsheaf preset. |
| `id` | `string` | yes | Stable identifier; bundled presets use `web`, `thumbnail`, `social`, `archive`. |
| `title` | `string` | yes |  |
| `description` | `string` | no |  |
| `pipeline` | [`Pipeline`](#pipeline) | yes |  |
| `naming` | [`Naming`](#naming) | no |  |

### Report

`report.json` in a ZIP download: one entry per file, in upload order, including failed and canceled files.

| Property | Type | Required | Description |
|---|---|---|---|
| `jobId` | [`Id`](#id) | yes |  |
| `createdAt` | `string` | yes |  |
| `files` | array of [`ReportEntry`](#reportentry) | yes |  |

### ReportEntry

| Property | Type | Required | Description |
|---|---|---|---|
| `index` | `integer` | yes |  |
| `source` | `string` | yes | Sanitized name of the upload or archive entry, with its folder. |
| `status` | [`FileStatus`](#filestatus) | yes |  |
| `inputBytes` | `integer` | yes |  |
| `inputFormat` | [`InputFormat`](#inputformat) | no |  |
| `output` | `string` | no | Path of the result inside the ZIP archive. |
| `outputBytes` | `integer` | no |  |
| `width` | `integer` | no |  |
| `height` | `integer` | no |  |
| `savedPercent` | `number` | no | Size reduction against the input; negative if the output is larger. |
| `error` | [`FileError`](#fileerror) | no |  |

### JobCreated

| Property | Type | Required | Description |
|---|---|---|---|
| `job` | [`Job`](#job) | yes |  |
| `token` | [`Token`](#token) | yes |  |

### JobStatus

- `open`: waiting for uploads; uploaded files are already processed.
- `processing`: all files uploaded, some still in progress.
- `done`: every file is `done`, `failed`, or `canceled`; the result
  can be downloaded.
- `canceled`: the job was canceled; nothing can be downloaded.

One of: `open`, `processing`, `done`, `canceled`.

### Job

| Property | Type | Required | Description |
|---|---|---|---|
| `id` | [`Id`](#id) | yes |  |
| `status` | [`JobStatus`](#jobstatus) | yes |  |
| `expectedFiles` | `integer` | yes |  |
| `createdAt` | `string` | yes |  |
| `expiresAt` | `string` | yes | The job and all its data are deleted at this time if not downloaded. |
| `files` | array of [`JobFile`](#jobfile) | yes | Uploaded files in upload order. |

### FileStatus

One of: `queued`, `processing`, `done`, `failed`, `canceled`.

### JobFile

| Property | Type | Required | Description |
|---|---|---|---|
| `id` | [`Id`](#id) | yes |  |
| `index` | `integer` | yes | Zero-based position within the job. |
| `kind` | `string` | yes | `image` for an image; `archive` for an uploaded ZIP archive, which is done once it has been unpacked into image files. One of: `image`, `archive`. |
| `source` | `string` | no | Sanitized name of the upload or archive entry, with its folder. |
| `archiveId` | [`Id`](#id) | no | The archive this image came from. |
| `downloaded` | `boolean` | no | The result was downloaded on its own and is no longer stored. |
| `status` | [`FileStatus`](#filestatus) | yes |  |
| `input` | [`ImageInfo`](#imageinfo) | no |  |
| `output` | [`OutputInfo`](#outputinfo) | no |  |
| `error` | [`FileError`](#fileerror) | no |  |

### ImageInfo

Facts about an uploaded image. Width and height are known once the header was read.

| Property | Type | Required | Description |
|---|---|---|---|
| `format` | [`InputFormat`](#inputformat) | yes |  |
| `bytes` | `integer` | yes |  |
| `width` | `integer` | no |  |
| `height` | `integer` | no |  |
| `pages` | `integer` | no | Number of frames or pages, for animated or multi-page images. |

### OutputInfo

Facts about the processed image.

| Property | Type | Required | Description |
|---|---|---|---|
| `format` | [`OutputFormat`](#outputformat) | yes |  |
| `bytes` | `integer` | yes |  |
| `width` | `integer` | yes |  |
| `height` | `integer` | yes |  |
| `pages` | `integer` | no |  |
| `name` | `string` | yes | Sanitized output file name, used in `Content-Disposition` and as the path inside a ZIP archive. |

### FileError

| Property | Type | Required | Description |
|---|---|---|---|
| `code` | `string` | yes | Stable machine-readable reason. One of: `unsupported_format`, `format_disabled`, `too_large`, `too_many_pixels`, `too_many_frames`, `decode_failed`, `encode_failed`, `target_size_unreachable`, `step_not_applicable`, `timeout`, `too_many_files`, `archive_invalid`, `archive_entry_unsupported`, `internal`. |
| `message` | `string` | yes | Human-readable explanation (English). Never contains file contents or paths. Codes for archives: `too_many_files` (the job or archive would exceed its file limit), `archive_invalid` (not a readable ZIP archive, or above `maxArchiveBytes`), `archive_entry_unsupported` (symbolic link, encrypted entry, or nested archive); entries that are too large or compressed too strongly fail with `too_large`. |

### FileUpload

| Property | Type | Required | Description |
|---|---|---|---|
| `file` | `string` | yes | The image or ZIP archive. |

### Problem

Error details (RFC 9457).

| Property | Type | Required | Description |
|---|---|---|---|
| `type` | `string` | yes | URI identifying the problem type; `about:blank` when no more specific type exists. |
| `title` | `string` | yes |  |
| `status` | `integer` | yes |  |
| `detail` | `string` | no |  |
| `code` | `string` | yes | Stable machine-readable error code, such as `pipeline_invalid` or `rate_limited`. |
| `errors` | array of [`ValidationError`](#validationerror) | no | Validation errors, one per invalid value. |

### ValidationError

| Property | Type | Required | Description |
|---|---|---|---|
| `pointer` | `string` | yes | JSON Pointer (RFC 6901) to the invalid value in the request body. |
| `message` | `string` | yes |  |

### Pipeline

Declarative processing pipeline (ADR 0009). The same document is used
by presets, the UI, and the API, and the server validates it on every
request.

Steps run in order; a step with `when` runs only if all of its
conditions hold for the current image. The output format is the
source format unless a `convert` step applies; HEIC/HEIF input
without a `convert` step is saved as JPEG.

The maximums in this schema are hard limits. The operator's limits
(`GET /capabilities`) may be lower and are enforced as well.

| Property | Type | Required | Description |
|---|---|---|---|
| `version` | `integer` | yes | Pipeline format version. One of: `1`. |
| `steps` | array of [`Step`](#step) | yes |  |
| `metadata` | [`MetadataPolicy`](#metadatapolicy) | no |  |
| `colorSpace` | `string` | no | `srgb` (default) converts to sRGB using the embedded ICC profile; `keep` leaves the color space unchanged. One of: `srgb`, `keep`. |

### MetadataPolicy

Metadata (EXIF, GPS, XMP, IPTC) is removed by default. `keep: true` copies it to the output where the format supports it; GPS data is removed unless `keepLocation` is also true.

| Property | Type | Required | Description |
|---|---|---|---|
| `keep` | `boolean` | no |  |
| `keepLocation` | `boolean` | no |  |

### Step

One pipeline step, selected by `op`.

Type: `object`.

### Condition

All given conditions must hold. Dimensions refer to the image as it is when the step runs. Fixed vocabulary, no expressions (ADR 0009).

| Property | Type | Required | Description |
|---|---|---|---|
| `widthGt` | `integer` | no |  |
| `widthLt` | `integer` | no |  |
| `heightGt` | `integer` | no |  |
| `heightLt` | `integer` | no |  |
| `longestEdgeGt` | `integer` | no |  |
| `sourceFormat` | array of [`InputFormat`](#inputformat) | no | The format of the uploaded file. |
| `orientation` | `string` | no |  One of: `landscape`, `portrait`, `square`. |

### AutoOrientStep

Rotates and flips the image according to its EXIF orientation tag.

| Property | Type | Required | Description |
|---|---|---|---|
| `op` | `string` | yes |  One of: `autoOrient`. |
| `when` | [`Condition`](#condition) | no |  |

### ResizeStep

| Property | Type | Required | Description |
|---|---|---|---|
| `op` | `string` | yes |  One of: `resize`. |
| `when` | [`Condition`](#condition) | no |  |
| `params` | [`ResizeParams`](#resizeparams) | yes |  |

### ResizeParams

Exactly one way of giving the size (enforced by the server):
- `width` and/or `height` in pixels,
- `longestEdge` in pixels,
- `percent` of the current size.

`mode` applies when both width and height are given:
- `fit`: fit inside the box, keeping the aspect ratio,
- `fill`: stretch to exactly the box,
- `crop`: cover the box and crop the overflow (using `gravity`).

| Property | Type | Required | Description |
|---|---|---|---|
| `width` | `integer` | no |  |
| `height` | `integer` | no |  |
| `longestEdge` | `integer` | no |  |
| `percent` | `number` | no |  |
| `mode` | `string` | no |  One of: `fit`, `fill`, `crop`. |
| `gravity` | [`Gravity`](#gravity) | no |  |
| `enlarge` | `boolean` | no | Allow the result to be larger than the current image. |

### Gravity

Anchor for cropping and placement. `attention` and `entropy` pick the most interesting region.

One of: `center`, `north`, `northeast`, `east`, `southeast`, `south`, `southwest`, `west`, `northwest`, `attention`, `entropy`.

### CropStep

| Property | Type | Required | Description |
|---|---|---|---|
| `op` | `string` | yes |  One of: `crop`. |
| `when` | [`Condition`](#condition) | no |  |
| `params` | [`CropParams`](#cropparams) | yes |  |

### CropParams

Either an aspect ratio (the largest region with that ratio, placed by `gravity`) or a rectangle in pixels, not both (enforced by the server). A rectangle that extends past the image is clipped.

| Property | Type | Required | Description |
|---|---|---|---|
| `aspectRatio` | [`AspectRatio`](#aspectratio) | no |  |
| `gravity` | [`Gravity`](#gravity) | no |  |
| `rect` | [`Rect`](#rect) | no |  |

### AspectRatio

| Property | Type | Required | Description |
|---|---|---|---|
| `width` | `integer` | yes |  |
| `height` | `integer` | yes |  |

### Rect

| Property | Type | Required | Description |
|---|---|---|---|
| `left` | `integer` | yes |  |
| `top` | `integer` | yes |  |
| `width` | `integer` | yes |  |
| `height` | `integer` | yes |  |

### RotateStep

| Property | Type | Required | Description |
|---|---|---|---|
| `op` | `string` | yes |  One of: `rotate`. |
| `when` | [`Condition`](#condition) | no |  |
| `params` | `object` | yes |  |

### FlipStep

| Property | Type | Required | Description |
|---|---|---|---|
| `op` | `string` | yes |  One of: `flip`. |
| `when` | [`Condition`](#condition) | no |  |
| `params` | `object` | yes |  |

### WatermarkStep

| Property | Type | Required | Description |
|---|---|---|---|
| `op` | `string` | yes |  One of: `watermark`. |
| `when` | [`Condition`](#condition) | no |  |
| `params` | [`WatermarkParams`](#watermarkparams) | yes |  |

### WatermarkParams

A text or an image placed on the picture. Text is plain text (no markup) rendered with the bundled font. An image watermark is a PNG or WebP, embedded as base64 and subject to `maxWatermarkBytes`; it is decoded in the worker sandbox like any upload, with at most 4096 x 4096 pixels. Exactly one of `text` and `image` is required, and `color` applies only to text (enforced by the server). The watermark is shrunk if it would not fit inside the margins. A watermark that cannot be used fails the file with `step_not_applicable`.

| Property | Type | Required | Description |
|---|---|---|---|
| `text` | `string` | no | One line of plain text without control characters, rendered with the bundled Go Bold font (Latin, Greek and Cyrillic; other scripts and emoji are not supported). |
| `color` | `string` | no | Text color as `#RRGGBB`. |
| `image` | `string` | no | Base64-encoded PNG or WebP (standard alphabet, with padding). |
| `size` | `number` | no | Watermark width as a percentage of the image width. |
| `position` | [`Gravity`](#gravity) | no |  |
| `margin` | `number` | no | Distance from the edge as a percentage of the shorter image side. |
| `opacity` | `number` | no |  |

### ConvertStep

Sets the output format and encoder options. If several apply, the last one wins.

| Property | Type | Required | Description |
|---|---|---|---|
| `op` | `string` | yes |  One of: `convert`. |
| `when` | [`Condition`](#condition) | no |  |
| `params` | [`ConvertParams`](#convertparams) | yes |  |

### ConvertParams

| Property | Type | Required | Description |
|---|---|---|---|
| `format` | [`OutputFormat`](#outputformat) | yes |  |
| `quality` | `integer` | no | Quality for lossy formats (jpeg, webp, avif, jxl, and pdf with a JPEG image). |
| `lossless` | `boolean` | no | Lossless mode for webp and jxl (png, gif, tiff, and bmp are always lossless). Not available for jpeg, avif, and pdf. |
| `optimize` | `boolean` | no | Spend more CPU time for a smaller file without changing pixels (PNG compression level and palette search, JPEG optimized Huffman tables, effort for webp, avif, and jxl). |
| `targetSizeKb` | `integer` | no | Largest acceptable output size. The quality is searched downward until the output fits; if it cannot fit, the file fails with `target_size_unreachable`. Not allowed together with `quality` or `lossless`. |
| `progressive` | `boolean` | no | Progressive JPEG or interlaced PNG/GIF. |
