// Simple JSON-based localization of bot texts.

import ru from "./locales/ru.json";
import en from "./locales/en.json";

/** Supported interface languages. */
export const LANG_RU = "ru";
export const LANG_EN = "en";

type Bundle = Record<string, string>;

const BUNDLES: Record<string, Bundle> = {
  [LANG_RU]: ru as Bundle,
  [LANG_EN]: en as Bundle,
};

/** Maps a Telegram language code to a supported locale. */
export function normalize(langCode: string | undefined): string {
  const code = (langCode ?? "").toLowerCase();
  if (code.startsWith(LANG_RU)) return LANG_RU;
  if (code.startsWith(LANG_EN)) return LANG_EN;
  return LANG_RU;
}

/**
 * Returns the localized string for key with {0}, {1}, ... placeholders
 * replaced by args. Unknown keys fall back to Russian, then to the key itself.
 */
export function t(lang: string, key: string, ...args: unknown[]): string {
  const bundle = BUNDLES[lang] ?? BUNDLES[LANG_RU]!;
  let text = bundle[key];
  if (text === undefined) {
    text = BUNDLES[LANG_RU]![key];
    if (text === undefined) {
      console.warn(`i18n: missing key ${key} (lang=${lang})`);
      return key;
    }
  }
  if (args.length === 0) return text;
  return text.replace(/\{(\d+)\}/g, (whole, index: string) => {
    const value = args[Number(index)];
    return value === undefined ? whole : String(value);
  });
}

/** Formats a date according to the locale's "time.format" pattern. */
export function formatTime(date: Date, lang: string): string {
  const pattern = t(lang, "time.format");
  const pad = (n: number, width = 2): string => String(n).padStart(width, "0");
  return pattern
    .replace(/YYYY/g, String(date.getFullYear()))
    .replace(/MM/g, pad(date.getMonth() + 1))
    .replace(/DD/g, pad(date.getDate()))
    .replace(/HH/g, pad(date.getHours()))
    .replace(/mm/g, pad(date.getMinutes()))
    .replace(/ss/g, pad(date.getSeconds()));
}
