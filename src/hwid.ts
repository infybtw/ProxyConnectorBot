// Stable device identifiers for subscriptions.

/**
 * Generates a new random HWID in the format used by Happ/INCY: an uppercase
 * UUID-like string (8-4-4-4-12). The value is generated once per origin and
 * stored in the database, so it stays stable across subscription refreshes.
 */
export function generateHWID(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  // Set UUID v4 version and variant bits.
  bytes[6] = (bytes[6]! & 0x0f) | 0x40;
  bytes[8] = (bytes[8]! & 0x3f) | 0x80;

  let hex = "";
  for (const byte of bytes) hex += byte.toString(16).padStart(2, "0");
  hex = hex.toUpperCase();
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20, 32)}`;
}
