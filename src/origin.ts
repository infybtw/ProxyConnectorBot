// Fetches subscriptions from provider (origin) servers, attaching the stored
// HWID so the provider treats the request as coming from the bound device.

import type { DeviceIdentity } from "./config";
import { HWID_MODE_QUERY, type Origin } from "./store";

/** A raw origin response. */
export interface OriginResult {
  statusCode: number;
  header: Headers;
  body: Uint8Array;
}

/** Fetches subscriptions from origin servers. */
export class OriginClient {
  constructor(
    private readonly timeoutMs: number,
    private readonly maxLen: number,
    private readonly device: DeviceIdentity,
  ) {}

  /**
   * Requests the subscription from the origin URL, attaching the origin's HWID
   * according to its delivery mode (header or query parameter) plus stable
   * device headers expected by Happ/INCY style panels.
   */
  async fetch(origin: Origin, externalSignal?: AbortSignal): Promise<OriginResult> {
    let target: URL;
    try {
      target = new URL(origin.url);
    } catch (err) {
      throw new Error(`origin: parse url: ${String(err)}`);
    }

    if (origin.hwidMode === HWID_MODE_QUERY) {
      target.searchParams.set(origin.hwidParam, origin.hwid);
    }

    const headers = new Headers();
    if (origin.hwidMode !== HWID_MODE_QUERY) {
      headers.set(origin.hwidParam, origin.hwid);
    }
    headers.set("x-device-os", this.device.os);
    headers.set("x-ver-os", this.device.osVersion);
    headers.set("x-device-model", this.device.model);
    headers.set("Accept", "*/*");
    if (this.device.userAgent !== "") {
      headers.set("User-Agent", this.device.userAgent);
    }

    const signal = externalSignal
      ? AbortSignal.any([externalSignal, AbortSignal.timeout(this.timeoutMs)])
      : AbortSignal.timeout(this.timeoutMs);

    let response: Response;
    try {
      response = await fetch(target, { method: "GET", headers, signal, redirect: "follow" });
    } catch (err) {
      throw new Error(`origin: request failed: ${String(err)}`);
    }

    const body = await readLimited(response, this.maxLen);
    return { statusCode: response.status, header: response.headers, body };
  }
}

/** Reads a response body, rejecting it when it exceeds maxLen bytes. */
async function readLimited(response: Response, maxLen: number): Promise<Uint8Array> {
  // Fast path: a declared length over the cap never needs to be read.
  const declared = Number(response.headers.get("content-length"));
  if (Number.isFinite(declared) && declared > maxLen) {
    throw new Error(`origin: response exceeds ${maxLen} bytes`);
  }
  if (response.body === null) return new Uint8Array();

  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      total += value.byteLength;
      if (total > maxLen) {
        await reader.cancel();
        throw new Error(`origin: response exceeds ${maxLen} bytes`);
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }

  const out = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return out;
}
