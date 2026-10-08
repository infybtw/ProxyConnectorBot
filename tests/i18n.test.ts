import { describe, expect, test } from "bun:test";
import { formatTime, normalize, t } from "../src/i18n";

describe("normalize", () => {
  test("maps language codes to supported locales", () => {
    expect(normalize("ru")).toBe("ru");
    expect(normalize("ru-RU")).toBe("ru");
    expect(normalize("en-US")).toBe("en");
    expect(normalize("de")).toBe("ru");
    expect(normalize(undefined)).toBe("ru");
  });
});

describe("t", () => {
  test("substitutes positional placeholders", () => {
    expect(t("en", "subs.title", 3)).toBe("📋 Your subscriptions (3):");
  });

  test("falls back to Russian for a missing English key", () => {
    const value = t("en", "lang.set");
    expect(value).toContain("English");
  });

  test("returns the key for an unknown key", () => {
    expect(t("ru", "does.not.exist")).toBe("does.not.exist");
  });
});

describe("formatTime", () => {
  test("honours the locale pattern", () => {
    const date = new Date(2026, 0, 2, 15, 4, 5);
    expect(formatTime(date, "ru")).toBe("02.01.2026 15:04");
    expect(formatTime(date, "en")).toBe("2026-01-02 15:04");
  });
});
