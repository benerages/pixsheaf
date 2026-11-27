// Formatting helpers that follow the current locale.

/**
 * Sizes in binary units with the familiar labels, so a configured limit of
 * 25 MiB reads as "25 MB", like in most file managers.
 */
export function formatBytes(bytes: number, locale: string): string {
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  const digits = unit === 0 || value >= 100 ? 0 : 1;
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: digits }).format(value)} ${units[unit]}`;
}

/** Relative change from before to after, such as -42 for 42 % smaller. */
export function changePercent(before: number, after: number): number {
  if (before <= 0) {
    return 0;
  }
  return Math.round(((after - before) / before) * 100);
}

export function formatPercent(value: number, locale: string): string {
  const sign = value > 0 ? "+" : "";
  return `${sign}${new Intl.NumberFormat(locale).format(value)} %`;
}
