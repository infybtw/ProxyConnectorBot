// URL helpers shared by the bot and the origin client.

import { HWID_MODE_HEADER, HWID_MODE_QUERY } from "./store";

/** Reports whether s looks like an absolute http(s) URL. */
export function isHttpUrl(s: string): boolean {
  try {
    const url = new URL(s.trim());
    return (url.protocol === "http:" || url.protocol === "https:") && url.host !== "";
  } catch {
    return false;
  }
}

/**
 * Guesses how the origin expects the HWID: if the URL already carries a
 * hwid-like query parameter we mirror it as a query parameter, otherwise we
 * use the x-hwid header (Happ/INCY style).
 */
export function detectHwidMode(rawUrl: string): { mode: string; param: string } {
  try {
    const url = new URL(rawUrl);
    for (const key of url.searchParams.keys()) {
      if (key.toLowerCase().includes("hwid")) {
        return { mode: HWID_MODE_QUERY, param: key };
      }
    }
  } catch {
    // Fall through to the default below.
  }
  return { mode: HWID_MODE_HEADER, param: "x-hwid" };
}

/** Derives a display name from the origin host. */
export function defaultName(rawUrl: string): string {
  try {
    const url = new URL(rawUrl);
    return url.host || "subscription";
  } catch {
    return "subscription";
  }
}
