import { describe, expect, test } from "bun:test";
import { generateHWID } from "../src/hwid";

const UUID_RE = /^[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$/;

describe("generateHWID", () => {
  test("produces unique uppercase UUID v4 values", () => {
    const seen = new Set<string>();
    for (let i = 0; i < 100; i++) {
      const id = generateHWID();
      expect(id).toMatch(UUID_RE);
      expect(seen.has(id)).toBe(false);
      seen.add(id);
    }
  });
});
