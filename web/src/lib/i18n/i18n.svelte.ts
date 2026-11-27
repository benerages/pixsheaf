// Minimal i18n without a dependency: English by default, German when the
// browser prefers it or the user picks it. The choice is kept in
// localStorage when available.
import { de, en, type MessageKey } from "./messages";

export type Locale = "en" | "de";
export const locales: { id: Locale; name: string }[] = [
  { id: "en", name: "English" },
  { id: "de", name: "Deutsch" },
];

const storageKey = "pixsheaf.locale";
const dictionaries: Record<Locale, Record<MessageKey, string>> = { en, de };

function initialLocale(): Locale {
  try {
    const stored = localStorage.getItem(storageKey);
    if (stored === "en" || stored === "de") {
      return stored;
    }
  } catch {
    // Storage may be unavailable.
  }
  return navigator.language.toLowerCase().startsWith("de") ? "de" : "en";
}

export const i18n = $state({ locale: initialLocale() });

export function setLocale(locale: Locale): void {
  i18n.locale = locale;
  document.documentElement.lang = locale;
  try {
    localStorage.setItem(storageKey, locale);
  } catch {
    // Storage may be unavailable.
  }
}

/** Translates key and fills {placeholders} from params. */
export function t(key: MessageKey, params: Record<string, string | number> = {}): string {
  const text = dictionaries[i18n.locale][key];
  return text.replace(/\{(\w+)\}/g, (match, name: string) =>
    name in params ? String(params[name]) : match,
  );
}

/** Translates an API error code, falling back to a generic message. */
export function errorText(code: string, detail = ""): string {
  const key = `error.${code}` as MessageKey;
  return key in en ? t(key, { detail }) : t("error.unknown");
}

/** The BCP 47 tag for number formatting. */
export function numberLocale(): string {
  return i18n.locale === "de" ? "de-CH" : "en-US";
}
