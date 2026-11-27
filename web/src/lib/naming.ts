// Client-side mirror of the naming rules in internal/jobs/names.go. The
// server stays authoritative; this only gives early feedback and an
// example while the user types.

export const placeholders = ["name", "index", "width", "height", "date", "ext"] as const;
export const maxPatternLength = 200;

const extensions: Record<string, string> = {
  jpeg: "jpg",
  png: "png",
  webp: "webp",
  gif: "gif",
  tiff: "tif",
  bmp: "bmp",
  avif: "avif",
  jxl: "jxl",
  pdf: "pdf",
};

export type PatternProblem =
  { kind: "length" } | { kind: "unmatched"; brace: "{" | "}" } | { kind: "unknown"; name: string };

/** Returns the first problem of a pattern, or null if it is valid. */
export function checkPattern(pattern: string): PatternProblem | null {
  if (pattern === "" || [...pattern].length > maxPatternLength) {
    return { kind: "length" };
  }
  let rest = pattern;
  for (;;) {
    const i = rest.search(/[{}]/);
    if (i < 0) {
      return null;
    }
    if (rest[i] === "}") {
      return { kind: "unmatched", brace: "}" };
    }
    const end = rest.indexOf("}", i);
    if (end < 0) {
      return { kind: "unmatched", brace: "{" };
    }
    const name = rest.slice(i + 1, end);
    if (!(placeholders as readonly string[]).includes(name)) {
      return { kind: "unknown", name };
    }
    rest = rest.slice(end + 1);
  }
}

export interface ExampleVars {
  name: string;
  index: number;
  total: number;
  width: number;
  height: number;
  format: string;
  date: Date;
}

/** Expands a valid pattern for one file, without the server's sanitizing. */
export function expandPattern(pattern: string, v: ExampleVars): string {
  const digits = String(Math.max(v.total, 1)).length;
  const values: Record<string, string> = {
    name: v.name,
    index: String(v.index).padStart(digits, "0"),
    width: String(v.width),
    height: String(v.height),
    date: v.date.toISOString().slice(0, 10),
    ext: extensions[v.format] ?? v.format,
  };
  const base = pattern.replace(/\{(\w+)\}/g, (match, key: string) => values[key] ?? match);
  return `${base}.${extensions[v.format] ?? v.format}`;
}

/** The base name of an uploaded path, without folders and extension. */
export function baseName(path: string): string {
  const file = path.split(/[\\/]/).pop() ?? path;
  const dot = file.lastIndexOf(".");
  return dot > 0 ? file.slice(0, dot) : file;
}
