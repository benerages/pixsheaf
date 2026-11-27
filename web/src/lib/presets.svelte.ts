// Bundled presets come from the server (GET /presets); custom presets are
// kept in this browser and exchanged as the same JSON document, so a
// preset gives the same result in the UI and through the API (ADR 0022).
import { getPresets, type Preset } from "./api/client";
import { parsePipeline } from "./pipeline";

const storageKey = "pixsheaf.presets";

function loadCustom(): Preset[] {
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) {
      return [];
    }
    const list: unknown = JSON.parse(raw);
    return Array.isArray(list) ? list.map(validPreset).filter((p): p is Preset => p !== null) : [];
  } catch {
    return [];
  }
}

/** Checks the shape of an imported or stored preset. */
export function validPreset(value: unknown): Preset | null {
  if (typeof value !== "object" || value === null) {
    return null;
  }
  const p = value as Partial<Preset>;
  if (
    p.format !== "pixsheaf-preset" ||
    typeof p.id !== "string" ||
    !/^[a-z0-9][a-z0-9-]{0,62}$/.test(p.id) ||
    typeof p.title !== "string" ||
    p.title.length < 1 ||
    p.title.length > 100 ||
    parsePipeline(JSON.stringify(p.pipeline)) === null
  ) {
    return null;
  }
  return p as Preset;
}

class Presets {
  bundled = $state<Preset[]>([]);
  custom = $state<Preset[]>(loadCustom());

  async load(): Promise<void> {
    try {
      this.bundled = await getPresets();
    } catch {
      this.bundled = [];
    }
  }

  find(id: string): Preset | undefined {
    return [...this.bundled, ...this.custom].find((p) => p.id === id);
  }

  save(preset: Preset): void {
    this.custom = [...this.custom.filter((p) => p.id !== preset.id), preset];
    this.persist();
  }

  remove(id: string): void {
    this.custom = this.custom.filter((p) => p.id !== id);
    this.persist();
  }

  private persist(): void {
    try {
      localStorage.setItem(storageKey, JSON.stringify($state.snapshot(this.custom)));
    } catch {
      // Storage may be unavailable; the preset lives until the page closes.
    }
  }
}

export const presets = new Presets();

/** A preset id from a title: lowercase letters, digits, and dashes. */
export function slug(title: string): string {
  const s = title
    .normalize("NFKD")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 50);
  return `custom-${s || "preset"}`;
}

/** Offers a preset as a JSON file download. */
export function exportPreset(preset: Preset): void {
  const blob = new Blob([JSON.stringify(preset, null, 2) + "\n"], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${preset.id}.pixsheaf.json`;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}

/** Reads a preset file; returns null if it is not a preset. */
export async function importPreset(file: File): Promise<Preset | null> {
  if (file.size > 2_000_000) {
    return null;
  }
  try {
    return validPreset(JSON.parse(await file.text()));
  } catch {
    return null;
  }
}
