# Architecture decision records

Each ADR records one decision with its context and consequences. Status values: Proposed (awaiting owner approval), Accepted, Superseded.

| ADR | Title | Status |
|---|---|---|
| [0001](0001-project-name.md) | Project name "Pixsheaf" | Accepted |
| [0002](0002-server-language.md) | Server language | Accepted |
| [0003](0003-libvips-go-binding.md) | libvips Go binding | Accepted |
| [0004](0004-libvips-build-and-worker-base-image.md) | libvips build and worker base image | Superseded by 0018 |
| [0005](0005-api-base-image.md) | API base image | Accepted |
| [0006](0006-worker-isolation.md) | Worker isolation | Accepted |
| [0007](0007-job-transport.md) | Job transport | Accepted |
| [0008](0008-libvips-hardening-configuration.md) | libvips hardening configuration | Accepted |
| [0009](0009-pipeline-format.md) | Pipeline format | Accepted |
| [0010](0010-frontend-stack.md) | Frontend stack | Accepted |
| [0011](0011-openapi-tooling.md) | OpenAPI tooling | Accepted |
| [0012](0012-api-resource-naming.md) | API resource naming | Accepted |
| [0013](0013-supply-chain-hardening.md) | Supply-chain hardening | Accepted |
| [0014](0014-no-antivirus-scanning-image-only-uploads.md) | No antivirus scanning; uploads restricted to images | Accepted |
| [0015](0015-anonymous-uploads.md) | Anonymous uploads with compensating controls | Accepted |
| [0016](0016-heic-in-default-worker.md) | HEIC/HEIF decoding in the default worker image | Accepted |
| [0017](0017-palette-quantization-library.md) | Palette quantization with quantizr instead of libimagequant | Accepted |
| [0018](0018-wolfi-worker-base.md) | Wolfi as the worker base; minimal runtime root filesystem | Accepted |
| [0019](0019-job-access-and-upload-flow.md) | Job access tokens and upload flow | Accepted |
| [0020](0020-text-watermark-rendering.md) | Text watermarks rendered in Go with the bundled Go fonts | Accepted |
| [0021](0021-bmp-and-pdf-in-go.md) | BMP and PDF handled in Go; no PDF library | Accepted |
| [0022](0022-batch-mode.md) | Batch mode: archives in the sandbox, fair queue, naming, presets | Accepted |
