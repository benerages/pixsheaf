# ADR 0009: Pipeline format

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Context

Presets, the UI, and the API all need the same format, and the result must be byte-identical whether a preset runs from the UI or the API.

## Options

(a) Declarative JSON validated by JSON Schema 2020-12. (b) A custom DSL. (c) Query parameters in the style of imagor's URLs.

## Decision

Option (a). A pipeline has the shape `{"version":1,"steps":[{"op":"resize","params":{...},"when":{...}}]}` and is validated against a closed schema (`additionalProperties:false`) that is embedded in the OpenAPI 3.1 spec. Conditions come from a fixed vocabulary (`width_gt`, `source_format`, `orientation`) with no expressions. Limits are configurable: maximum number of steps, maximum output dimensions and pixels, maximum watermark size and bytes, and maximum rename-pattern length. The server re-validates every request regardless of what the UI did.

## Consequences

One validator serves presets, the UI, and the API. Presets can be exported and imported as plain JSON. Adding new operations requires bumping the schema version. Fuzz the parser.

## Amendment 2026-11-27: validation without a JSON Schema library (Phase 3)

`internal/pipeline` decodes the document into typed Go structures with unknown fields rejected at every level and no trailing data allowed, then checks every range, enum, and the rules that the schema states only in prose (mutually exclusive parameters, operator limits). This accepts exactly the closed schema in `api/openapi.yaml` without adding a JSON Schema validator as a dependency. Errors carry JSON Pointers for the API's problem details. The API and the job process use the same parser, and the job process validates again (defense in depth). The output format is chosen by `convert` steps; without one the source format is kept and HEIF becomes JPEG (ADR 0019).
