// Combines several subscription bodies into one. Subscriptions are lists of
// share links, either plain text or base64 encoded.

/** One successful origin response. */
export interface Part {
  body: Uint8Array;
  header: Headers;
}

/** The merged subscription. */
export interface MergeResult {
  body: Uint8Array;
  header: Headers;
}

/** Thrown when there is nothing to merge. */
export class EmptyBundleError extends Error {
  constructor() {
    super("bundle: no subscription parts");
    this.name = "EmptyBundleError";
  }
}

// Traffic counters of subscription-userinfo that are summed across parts. The
// earliest non-zero expire wins.
const USERINFO_KEYS = ["upload", "download", "total"] as const;

/**
 * Concatenates the share links of all parts, dropping duplicates while keeping
 * order. The output is base64 when any part was base64 encoded. Headers come
 * from the first part, with traffic counters summed.
 */
export function merge(parts: Part[]): MergeResult {
  if (parts.length === 0) throw new EmptyBundleError();

  const out: string[] = [];
  const seen = new Set<string>();
  let encoded = false;
  const info: string[] = [];

  for (const part of parts) {
    const { lines, wasEncoded } = splitLines(part.body);
    encoded = encoded || wasEncoded;
    for (const line of lines) {
      if (!seen.has(line)) {
        seen.add(line);
        out.push(line);
      }
    }
    const userinfo = part.header.get("Subscription-Userinfo");
    if (userinfo) info.push(userinfo);
  }

  let body = out.join("\n");
  if (encoded) body = Buffer.from(body, "utf8").toString("base64");

  const header = new Headers(parts[0]!.header);
  if (info.length > 0) {
    header.set("Subscription-Userinfo", mergeUserinfo(info));
  }
  return { body: new TextEncoder().encode(body), header };
}

/** Returns the non-empty trimmed lines and whether the body was base64 encoded. */
function splitLines(body: Uint8Array): { lines: string[]; wasEncoded: boolean } {
  const text = new TextDecoder().decode(body).trim();
  const decoded = decodeBase64(text);
  if (decoded !== null) return { lines: toLines(decoded), wasEncoded: true };
  return { lines: toLines(text), wasEncoded: false };
}

/**
 * Decodes a base64 subscription body. Anything that does not decode to text
 * with share links is treated as plain text.
 */
export function decodeBase64(text: string): string | null {
  const compact = text.replace(/\s+/g, "");
  if (compact === "") return null;

  const variants: Array<{ encoding: BufferEncoding; pattern: RegExp }> = [
    { encoding: "base64", pattern: /^[A-Za-z0-9+/]*={0,2}$/ },
    { encoding: "base64url", pattern: /^[A-Za-z0-9_-]*={0,2}$/ },
  ];

  for (const { encoding, pattern } of variants) {
    if (!pattern.test(compact)) continue;
    let buf: Buffer;
    try {
      buf = Buffer.from(compact, encoding);
    } catch {
      continue;
    }
    // Buffer.from is lenient: verify a round-trip ignoring padding.
    const normalized = compact.replace(/=+$/, "");
    if (buf.toString(encoding).replace(/=+$/, "") !== normalized) continue;
    const decoded = decodeUtf8Strict(buf);
    if (decoded !== null && decoded.includes("://")) return decoded;
  }
  return null;
}

/** Decodes UTF-8, returning null for invalid byte sequences. */
function decodeUtf8Strict(buf: Buffer): string | null {
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(buf);
  } catch {
    return null;
  }
}

function toLines(text: string): string[] {
  const out: string[] = [];
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (trimmed !== "") out.push(trimmed);
  }
  return out;
}

/**
 * Sums traffic counters and keeps the earliest expiry from subscription-userinfo
 * header values such as "upload=1; download=2; total=3; expire=4".
 */
export function mergeUserinfo(values: string[]): string {
  const sums: Record<string, number> = {};
  const present: Record<string, boolean> = {};
  let expire = 0;

  for (const value of values) {
    for (const field of value.split(";")) {
      const eq = field.indexOf("=");
      if (eq === -1) continue;
      const key = field.slice(0, eq).trim().toLowerCase();
      const rawValue = field.slice(eq + 1).trim();
      const n = Number.parseInt(rawValue, 10);
      if (!Number.isFinite(n)) continue;

      if (key === "expire") {
        if (n > 0 && (expire === 0 || n < expire)) expire = n;
      } else if ((USERINFO_KEYS as readonly string[]).includes(key)) {
        sums[key] = (sums[key] ?? 0) + n;
        present[key] = true;
      }
    }
  }

  const parts: string[] = [];
  for (const key of USERINFO_KEYS) {
    if (present[key]) parts.push(`${key}=${sums[key]}`);
  }
  if (expire > 0) parts.push(`expire=${expire}`);
  return parts.join("; ");
}
