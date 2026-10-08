// QR code rendering: qrcode draws the bitmap, Bun.Image re-encodes it as a small PNG.

import QRCode from "qrcode";

/** Side length in pixels of the rendered QR code. */
export const QR_SIZE = 512;

/** Encodes text into a PNG image suitable for Telegram's sendPhoto. */
export async function renderQrPng(text: string): Promise<Uint8Array> {
  const raw = await QRCode.toBuffer(text, {
    type: "png",
    errorCorrectionLevel: "M",
    margin: 2,
    width: QR_SIZE,
  });
  return new Bun.Image(raw).png({ palette: true, colors: 2 }).bytes();
}
