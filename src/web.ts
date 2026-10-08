// Serves subscriptions over HTTP on our own domain.

import { Elysia } from "elysia";
import { createHash } from "node:crypto";
import { log } from "./log";
import { merge, type Part } from "./merge";
import type { OriginClient, OriginResult } from "./origin";
import { NotFoundError, type DeviceInfo, type Store, type Subscription } from "./store";

// Headers that must not be forwarded, see RFC 7230 section 6.1.
const HOP_BY_HOP_HEADERS = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
  // Set by us or computed by the runtime:
  "content-length",
  "set-cookie",
  // Identity headers we never leak to clients:
  "x-hwid",
  "x-device-os",
  "x-ver-os",
  "x-device-model",
]);

// Metadata limits protect the database from oversized or abusive headers.
const MAX_HWID_LEN = 128;
const MAX_UA_LEN = 512;
const MAX_OS_LEN = 64;
const MAX_MODEL_LEN = 128;
const MAX_IP_LEN = 64;

interface FetchResult {
  res?: OriginResult;
  err?: unknown;
}

/** Creates the Elysia app with /healthz and /s/:token. */
export function createServer(store: Store, origin: OriginClient) {
  return new Elysia({ name: "ProxyConnectorBot" })
    .get("/healthz", async () => {
      try {
        await store.ping();
        return text("ok", 200);
      } catch (err) {
        log.warn("web: healthcheck failed", { err });
        return text("db unavailable", 503);
      }
    })
    .get("/s/:token", async ({ params, request, query, server }) => {
      const token = params.token;

      let sub: Subscription;
      try {
        sub = await store.getByToken(token);
      } catch (err) {
        if (err instanceof NotFoundError) return text("subscription not found", 404);
        log.error("web: lookup failed", { token, err });
        return text("internal error", 500);
      }
      if (sub.origins.length === 0) return text("subscription has no origins", 404);

      // Best effort: remember which device fetched the subscription, even if
      // the origin requests below fail.
      try {
        await store.touchDevice(sub.id, deviceInfo(request, query, server));
      } catch (err) {
        log.warn("web: record device failed", { sub_id: sub.id, err });
      }

      const results = await fetchOrigins(origin, sub);

      if (sub.origins.length === 1) {
        const result = results[0]!;
        if (result.err !== undefined || result.res === undefined) {
          return text("origin unavailable", 502);
        }
        log.info("web: subscription served", {
          sub_id: sub.id,
          status: result.res.statusCode,
          size: result.res.body.byteLength,
        });
        return new Response(result.res.body, {
          status: result.res.statusCode,
          headers: filterHeaders(result.res.header),
        });
      }

      const parts: Part[] = [];
      for (const result of results) {
        if (result.err !== undefined || result.res === undefined) continue;
        if (result.res.statusCode !== 200) {
          log.warn("web: origin returned non-200", { sub_id: sub.id, status: result.res.statusCode });
          continue;
        }
        parts.push({ body: result.res.body, header: result.res.header });
      }
      if (parts.length === 0) return text("origin unavailable", 502);

      let merged;
      try {
        merged = merge(parts);
      } catch (err) {
        log.error("web: merge failed", { sub_id: sub.id, err });
        return text("origin unavailable", 502);
      }

      log.info("web: subscription served", {
        sub_id: sub.id,
        origins: sub.origins.length,
        merged: parts.length,
        size: merged.body.byteLength,
      });

      const headers = filterHeaders(merged.header);
      headers.set("Profile-Title", sub.name);
      return new Response(merged.body, { status: 200, headers });
    });
}

/** Requests every origin of the subscription concurrently, keeping their order. */
async function fetchOrigins(origin: OriginClient, sub: Subscription): Promise<FetchResult[]> {
  return await Promise.all(
    sub.origins.map(async (o) => {
      try {
        return { res: await origin.fetch(o) } satisfies FetchResult;
      } catch (err) {
        log.error("web: origin fetch failed", { sub_id: sub.id, origin_id: o.id, err });
        return { err } satisfies FetchResult;
      }
    }),
  );
}

/** Passes origin headers to the client, minus hop-by-hop and identity headers. */
function filterHeaders(header: Headers): Headers {
  const out = new Headers();
  header.forEach((value, key) => {
    if (HOP_BY_HOP_HEADERS.has(key.toLowerCase())) return;
    out.append(key, value);
  });
  return out;
}

/** Extracts the identity of the client that made the request. */
function deviceInfo(
  request: Request,
  query: Record<string, string | undefined>,
  server: { requestIP?: (req: Request) => { address?: string } | null } | null,
): DeviceInfo {
  const hwid = truncate(firstNonEmpty(request.headers.get("x-hwid"), query["hwid"]), MAX_HWID_LEN);
  const userAgent = truncate(request.headers.get("user-agent") ?? "", MAX_UA_LEN);
  const os = truncate(request.headers.get("x-device-os") ?? "", MAX_OS_LEN);
  const osVersion = truncate(request.headers.get("x-ver-os") ?? "", MAX_OS_LEN);
  const model = truncate(request.headers.get("x-device-model") ?? "", MAX_MODEL_LEN);
  const ip = truncate(clientIp(request, server), MAX_IP_LEN);

  const key = hwid === "" ? `ua:${fingerprint(userAgent, os, osVersion, model)}` : `hwid:${hwid}`;
  return { key, hwid, userAgent, os, osVersion, model, ip };
}

/** Prefers the X-Forwarded-For address set by the reverse proxy. */
function clientIp(
  request: Request,
  server: { requestIP?: (req: Request) => { address?: string } | null } | null,
): string {
  const xff = request.headers.get("x-forwarded-for");
  if (xff) {
    const first = xff.split(",")[0]?.trim();
    if (first) return first;
  }
  return server?.requestIP?.(request)?.address ?? "";
}

/** Builds a short stable id from device metadata for clients without a HWID. */
function fingerprint(...parts: string[]): string {
  return createHash("sha256").update(parts.join("|")).digest("hex").slice(0, 16);
}

function firstNonEmpty(...values: Array<string | null | undefined>): string {
  for (const value of values) {
    if (value) return value;
  }
  return "";
}

function truncate(s: string, n: number): string {
  return s.length <= n ? s : s.slice(0, n);
}

function text(body: string, status: number): Response {
  return new Response(body, {
    status,
    headers: { "content-type": "text/plain; charset=utf-8" },
  });
}
