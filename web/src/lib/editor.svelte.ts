// The settings shared by the single-image and the batch view. The pipeline
// is derived from the quick settings unless a preset or the advanced editor
// set it directly.
import type { Naming, Pipeline, Preset } from "./api/client";
import { checkPattern } from "./naming";
import {
  buildPipeline,
  defaultSettings,
  suggestResizeValue,
  type ResizeMode,
  type Settings,
  type Size,
} from "./pipeline";

export class Editor {
  settings = $state<Settings>({ ...defaultSettings });
  /** A pipeline from a preset or the advanced editor, or null. */
  custom = $state<Pipeline | null>(null);
  presetId = $state("");
  pattern = $state("{name}");
  keepFolders = $state(true);
  /** Size of the previewed source image, once known; drives resize suggestions. */
  source = $state<Size | null>(null);

  pipeline = $derived<Pipeline>(this.custom ?? buildPipeline(this.settings));
  naming = $derived<Naming>({ pattern: this.pattern, keepFolders: this.keepFolders });
  /** The first problem of the name pattern, or null. */
  patternProblem = $derived(checkPattern(this.pattern));

  /** Switches the resize mode and suggests a value that fits the mode. */
  setResize(mode: ResizeMode): void {
    this.settings.resize = mode;
    if (mode !== "none") {
      this.settings.resizeValue = suggestResizeValue(mode, this.source);
    }
  }

  applyPreset(preset: Preset): void {
    this.custom = structuredClone($state.snapshot(preset.pipeline)) as Pipeline;
    this.presetId = preset.id;
    this.pattern = preset.naming?.pattern ?? "{name}";
    if (preset.naming?.keepFolders !== undefined) {
      this.keepFolders = preset.naming.keepFolders;
    }
  }

  setCustom(pipeline: Pipeline): void {
    this.custom = pipeline;
    this.presetId = "";
  }

  useQuickSettings(): void {
    this.custom = null;
    this.presetId = "";
  }

  /** The current settings as a preset document for export. */
  toPreset(id: string, title: string): Preset {
    return {
      format: "pixsheaf-preset",
      id,
      title,
      pipeline: $state.snapshot(this.pipeline) as Pipeline,
      naming: $state.snapshot(this.naming) as Naming,
    };
  }
}
