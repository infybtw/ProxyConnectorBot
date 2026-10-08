import { describe, expect, test } from "bun:test";
import { QR_SIZE, renderQrPng } from "../src/qr";

const PNG_SIGNATURE = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];

describe("renderQrPng", () => {
  test("returns a square PNG of the configured size", async () => {
    const png = await renderQrPng("https://example.com/s/abc123");
    expect(Array.from(png.slice(0, 8))).toEqual(PNG_SIGNATURE);

    const meta = await new Bun.Image(png).metadata();
    expect(meta.format).toBe("png");
    expect(meta.width).toBe(QR_SIZE);
    expect(meta.height).toBe(QR_SIZE);
  });

  test("is deterministic for the same payload", async () => {
    const a = await renderQrPng("https://example.com/s/abc123");
    const b = await renderQrPng("https://example.com/s/abc123");
    expect(Buffer.from(a).equals(Buffer.from(b))).toBe(true);
  });
});
