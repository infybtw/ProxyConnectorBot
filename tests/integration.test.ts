// Integration tests. They require TEST_DATABASE_URL pointing at a disposable
// Postgres; without it the whole suite is skipped.

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { SQL } from "bun";
import { migrate } from "../src/migrate";
import { OriginClient } from "../src/origin";
import { createServer } from "../src/web";
import { LastOriginError, NotFoundError, Store } from "../src/store";

const DSN = Bun.env.TEST_DATABASE_URL;
const device = { os: "android", osVersion: "14", model: "Pixel 7", userAgent: "it" };

const describeIf = DSN ? describe : describe.skip;

describeIf("integration", () => {
  let sql: SQL;
  let store: Store;
  const servers: Array<{ stop: (force?: boolean) => void }> = [];

  beforeAll(async () => {
    sql = new SQL(DSN!);
    await migrate(sql);
    store = new Store(sql);
  });

  afterAll(async () => {
    for (const server of servers) server.stop(true);
    await sql.close();
  });

  function originServer(respond: (req: Request) => Response): string {
    const server = Bun.serve({ port: 0, fetch: (req) => respond(req) });
    servers.push(server);
    return `http://127.0.0.1:${server.port}`;
  }

  async function cleanupUser(userId: number): Promise<void> {
    await sql`DELETE FROM subscriptions WHERE user_id = ${userId}`;
    await sql`DELETE FROM users WHERE tg_id = ${userId}`;
  }

  test("serves a single origin as a transparent passthrough and tracks the device", async () => {
    const wantHWID = "270DD26E-160D-4257-B8AC-654800E12F24";
    const originUrl = originServer((req) => {
      if (req.headers.get("x-hwid") !== wantHWID) {
        return new Response("missing hwid", { status: 403 });
      }
      return new Response("vless://example", {
        status: 200,
        headers: { "Content-Type": "text/plain", "Profile-Title": "Test Sub" },
      });
    });

    const userId = 424242;
    await cleanupUser(userId);
    await store.upsertUser(userId, "ru");
    const sub = await store.createSubscription({
      userId,
      name: "it-test",
      origins: [{ url: originUrl, hwid: wantHWID, hwidMode: "header", hwidParam: "x-hwid" }],
    });

    const oc = new OriginClient(5_000, 1 << 20, device);
    const app = createServer(store, oc);

    const health = await app.handle(new Request("http://localhost/healthz"));
    expect(health.status).toBe(200);
    expect(await health.text()).toBe("ok");

    const resp = await app.handle(
      new Request(`http://localhost/s/${sub.token}`, {
        headers: { "User-Agent": "v2rayNG/1.8.5", "X-Forwarded-For": "203.0.113.7" },
      }),
    );
    expect(resp.status).toBe(200);
    expect(await resp.text()).toBe("vless://example");
    expect(resp.headers.get("profile-title")).toBe("Test Sub");
    expect(resp.headers.get("x-hwid")).toBeNull();

    const notFound = await app.handle(new Request("http://localhost/s/does-not-exist"));
    expect(notFound.status).toBe(404);

    const devices = await store.listDevices(sub.id, 10);
    expect(devices).toHaveLength(1);
    expect(devices[0]!.hwid).toBe("");
    expect(devices[0]!.userAgent).toBe("v2rayNG/1.8.5");
    expect(devices[0]!.ip).toBe("203.0.113.7");
    expect(devices[0]!.requests).toBe(1);

    // A client that sends a HWID is tracked separately; repeat requests bump the counter.
    for (let i = 0; i < 2; i++) {
      const r = await app.handle(
        new Request(`http://localhost/s/${sub.token}`, {
          headers: { "x-hwid": wantHWID, "User-Agent": "Happ/2.4.1" },
        }),
      );
      expect(r.status).toBe(200);
      await r.text();
    }
    const after = await store.listDevices(sub.id, 10);
    expect(after).toHaveLength(2);
    const hwidDevice = after.find((d) => d.hwid === wantHWID);
    expect(hwidDevice).toBeDefined();
    expect(hwidDevice!.requests).toBe(2);
    expect(hwidDevice!.userAgent).toBe("Happ/2.4.1");

    await cleanupUser(userId);
  });

  test("merges several origins and enforces origin invariants", async () => {
    const hwidA = "AAAA-1111";
    const hwidB = "BBBB-2222";
    const originA = originServer((req) => {
      if (req.headers.get("x-hwid") !== hwidA) return new Response(null, { status: 403 });
      return new Response(Buffer.from("vless://a\nvless://shared", "utf8").toString("base64"), {
        status: 200,
        headers: { "Subscription-Userinfo": "upload=1; download=2; total=10; expire=5000" },
      });
    });
    const originB = originServer((req) => {
      if (req.headers.get("x-hwid") !== hwidB) return new Response(null, { status: 403 });
      return new Response("trojan://b\nvless://shared", {
        status: 200,
        headers: { "Subscription-Userinfo": "upload=3; download=4; total=20; expire=4000" },
      });
    });

    const userId = 434343;
    await cleanupUser(userId);
    await store.upsertUser(userId, "ru");
    const sub = await store.createSubscription({
      userId,
      name: "multi",
      origins: [
        { url: originA, hwid: hwidA, hwidMode: "header", hwidParam: "x-hwid" },
        { url: originB, hwid: hwidB, hwidMode: "header", hwidParam: "x-hwid" },
      ],
    });
    expect(sub.origins).toHaveLength(2);
    expect(sub.origins[0]!.id).toBeGreaterThan(0);

    const oc = new OriginClient(5_000, 1 << 20, device);
    const app = createServer(store, oc);

    const resp = await app.handle(new Request(`http://localhost/s/${sub.token}`));
    const body = await resp.text();
    expect(Buffer.from(body, "base64").toString("utf8")).toBe("vless://a\nvless://shared\ntrojan://b");
    expect(resp.headers.get("subscription-userinfo")).toBe("upload=4; download=6; total=30; expire=4000");
    expect(resp.headers.get("profile-title")).toBe("multi");
    expect(resp.headers.get("x-hwid")).toBeNull();

    expect(await store.listDevices(sub.id, 10)).toHaveLength(1);

    // Removing one origin leaves the other served as a passthrough.
    await store.deleteOrigin(userId, sub.origins[0]!.id);
    const single = await app.handle(new Request(`http://localhost/s/${sub.token}`));
    expect(await single.text()).toBe("trojan://b\nvless://shared");

    // The last origin cannot be removed.
    await expect(store.deleteOrigin(userId, sub.origins[1]!.id)).rejects.toBeInstanceOf(LastOriginError);

    // Adding to a foreign or missing subscription is rejected.
    await expect(
      store.addOrigin(userId, sub.id + 1000, { url: originA, hwid: hwidA, hwidMode: "header", hwidParam: "x-hwid" }),
    ).rejects.toBeInstanceOf(NotFoundError);
    await store.addOrigin(userId, sub.id, { url: originA, hwid: hwidA, hwidMode: "header", hwidParam: "x-hwid" });
    const reloaded = await store.getSubscription(userId, sub.id);
    expect(reloaded.origins).toHaveLength(2);

    await cleanupUser(userId);
  });
});
