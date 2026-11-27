// Builds pipeline documents (ADR 0009) from the quick settings of the UI.
// The pipeline JSON is the single source of truth: quick settings produce
// it, presets replace it, and the advanced editor edits it directly.
import type { Pipeline } from "./api/client";

export type OutputFormat =
  "keep" | "jpeg" | "png" | "webp" | "avif" | "jxl" | "gif" | "tiff" | "bmp" | "pdf";

export const outputFormats: OutputFormat[] = [
  "keep",
  "jpeg",
  "png",
  "webp",
  "avif",
  "jxl",
  "gif",
  "tiff",
  "bmp",
  "pdf",
];

/** Formats with a quality setting. */
export const lossyFormats = new Set<OutputFormat>(["jpeg", "webp", "avif", "jxl", "pdf"]);
/** Formats with an optional lossless mode. */
export const losslessFormats = new Set<OutputFormat>(["webp", "jxl"]);

export type ResizeMode = "none" | "width" | "height" | "longest" | "percent";
export type Aspect = "none" | "1:1" | "4:3" | "3:2" | "16:9" | "4:5";
export type Gravity =
  | "north"
  | "northeast"
  | "east"
  | "southeast"
  | "south"
  | "southwest"
  | "west"
  | "northwest"
  | "center";

export interface Settings {
  format: OutputFormat;
  /** "quality" uses quality; "size" uses targetKb; "auto" uses neither. */
  compression: "auto" | "quality" | "size";
  quality: number;
  targetKb: number;
  lossless: boolean;
  resize: ResizeMode;
  resizeValue: number;
  crop: Aspect;
  rotate: 0 | 90 | 180 | 270;
  flip: "none" | "horizontal" | "vertical";
  keepMetadata: boolean;
  watermark: boolean;
  watermarkText: string;
  watermarkPosition: Gravity;
  watermarkOpacity: number;
}

export const defaultSettings: Settings = {
  format: "keep",
  compression: "auto",
  quality: 80,
  targetKb: 500,
  lossless: false,
  resize: "none",
  resizeValue: 1920,
  crop: "none",
  rotate: 0,
  flip: "none",
  keepMetadata: false,
  watermark: false,
  watermarkText: "",
  watermarkPosition: "southeast",
  watermarkOpacity: 0.5,
};

type Step = Pipeline["steps"][number];

function encoderParams(s: Settings, format: OutputFormat): Record<string, unknown> {
  const params: Record<string, unknown> = { format };
  if (losslessFormats.has(format) && s.lossless) {
    params.lossless = true;
  } else if (s.compression === "quality" && lossyFormats.has(format)) {
    params.quality = s.quality;
  } else if (s.compression === "size") {
    params.targetSizeKb = s.targetKb;
  }
  return params;
}

/** The pipeline for the quick settings. */
export function buildPipeline(s: Settings): Pipeline {
  const steps: Step[] = [{ op: "autoOrient" } as Step];
  if (s.crop !== "none") {
    const [w, h] = s.crop.split(":").map(Number);
    steps.push({ op: "crop", params: { aspectRatio: { width: w, height: h } } } as Step);
  }
  if (s.resize !== "none" && s.resizeValue > 0) {
    const key = { width: "width", height: "height", longest: "longestEdge", percent: "percent" }[
      s.resize
    ];
    steps.push({
      op: "resize",
      params: { [key]: s.resizeValue, mode: "fit", enlarge: false },
    } as Step);
  }
  if (s.rotate !== 0) {
    steps.push({ op: "rotate", params: { angle: s.rotate } } as Step);
  }
  if (s.flip !== "none") {
    steps.push({ op: "flip", params: { direction: s.flip } } as Step);
  }
  if (s.watermark && s.watermarkText.trim() !== "") {
    steps.push({
      op: "watermark",
      params: {
        text: s.watermarkText.trim(),
        position: s.watermarkPosition,
        opacity: s.watermarkOpacity,
      },
    } as Step);
  }
  if (s.format !== "keep") {
    steps.push({ op: "convert", params: encoderParams(s, s.format) } as Step);
  } else if (s.compression !== "auto") {
    // Keep each file's format but apply the compression: one conditional
    // step per source format (HEIC and HEIF become JPEG, as without a
    // convert step).
    const same: [string[], OutputFormat][] = [
      [["jpeg", "heif"], "jpeg"],
      [["webp"], "webp"],
      [["avif"], "avif"],
      [["jxl"], "jxl"],
      [["png"], "png"],
    ];
    for (const [sources, format] of same) {
      if (s.compression === "quality" && !lossyFormats.has(format)) {
        continue;
      }
      steps.push({
        op: "convert",
        when: { sourceFormat: sources },
        params: encoderParams(s, format),
      } as Step);
    }
  }
  const pipeline: Pipeline = { version: 1, colorSpace: "srgb", steps };
  if (s.keepMetadata) {
    pipeline.metadata = { keep: true, keepLocation: false };
  }
  return pipeline;
}

/** Parses an edited pipeline document; the server validates it in full. */
export function parsePipeline(text: string): Pipeline | null {
  try {
    const value: unknown = JSON.parse(text);
    if (
      typeof value === "object" &&
      value !== null &&
      (value as { version?: unknown }).version === 1 &&
      Array.isArray((value as { steps?: unknown }).steps)
    ) {
      return value as Pipeline;
    }
  } catch {
    // Invalid JSON.
  }
  return null;
}
