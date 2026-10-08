// Postgres persistence for users, subscriptions and devices (bun.SQL).

import type { SQL } from "bun";
import { sql as sqlList } from "bun";

/** HWID delivery modes. */
export const HWID_MODE_HEADER = "header";
export const HWID_MODE_QUERY = "query";

/** A Telegram user of the bot. */
export interface User {
  tgId: number;
  lang: string;
}

/** An origin is a provider subscription URL bound to a HWID. */
export interface Origin {
  id: number;
  subscriptionId: number;
  url: string;
  hwid: string;
  hwidMode: string;
  hwidParam: string;
  /** Whether the origin takes part in subscription serving. */
  enabled: boolean;
  createdAt: Date;
}

/** An origin as provided when creating a subscription or adding an origin. */
export interface NewOrigin {
  url: string;
  hwid: string;
  hwidMode: string;
  hwidParam: string;
}

/** A public subscription proxied through our domain. */
export interface Subscription {
  id: number;
  userId: number;
  name: string;
  token: string;
  createdAt: Date;
  origins: Origin[];
}

/** A subscription as provided when creating it. */
export interface NewSubscription {
  userId: number;
  name: string;
  origins: NewOrigin[];
}

/** Metadata captured from a client that fetched a subscription. */
export interface DeviceInfo {
  /** Groups requests from the same device (HWID or a metadata fingerprint). */
  key: string;
  hwid: string;
  userAgent: string;
  os: string;
  osVersion: string;
  model: string;
  ip: string;
}

/** A stored device record. */
export interface Device {
  deviceKey: string;
  hwid: string;
  userAgent: string;
  os: string;
  osVersion: string;
  model: string;
  ip: string;
  requests: number;
  firstSeen: Date;
  lastSeen: Date;
}

/** Thrown when the requested row does not exist. */
export class NotFoundError extends Error {
  constructor(message = "store: not found") {
    super(message);
    this.name = "NotFoundError";
  }
}

/** Thrown when deleting the only origin of a subscription. */
export class LastOriginError extends Error {
  constructor(message = "store: last origin of subscription") {
    super(message);
    this.name = "LastOriginError";
  }
}

type Row = Record<string, any>;

const num = (value: unknown): number => (typeof value === "number" ? value : Number(value));
const date = (value: unknown): Date => (value instanceof Date ? value : new Date(String(value)));

function mapOrigin(row: Row): Origin {
  return {
    id: num(row.id),
    subscriptionId: num(row.subscription_id),
    url: String(row.origin_url),
    hwid: String(row.hwid),
    hwidMode: String(row.hwid_mode),
    hwidParam: String(row.hwid_param),
    enabled: row.enabled === true,
    createdAt: date(row.created_at),
  };
}

function mapSubscription(row: Row): Subscription {
  return {
    id: num(row.id),
    userId: num(row.user_id),
    name: String(row.name),
    token: String(row.token),
    createdAt: date(row.created_at),
    origins: [],
  };
}

/** Store wraps a bun.SQL connection. */
export class Store {
  constructor(private readonly sql: SQL) {}

  /** Checks database connectivity. */
  async ping(): Promise<void> {
    await this.sql`SELECT 1`;
  }

  /** Creates the user or keeps the existing row untouched. */
  async upsertUser(tgId: number, lang: string): Promise<void> {
    await this.sql`INSERT INTO users (tg_id, lang) VALUES (${tgId}, ${lang})
                   ON CONFLICT (tg_id) DO NOTHING`;
  }

  /** Returns the user row or null. */
  async getUser(tgId: number): Promise<User | null> {
    const rows = (await this.sql`SELECT tg_id, lang FROM users WHERE tg_id = ${tgId}`) as Row[];
    const row = rows[0];
    if (row === undefined) return null;
    return { tgId: num(row.tg_id), lang: String(row.lang) };
  }

  /** Saves the interface language of the user. */
  async setLang(tgId: number, lang: string): Promise<void> {
    await this.sql`UPDATE users SET lang = ${lang} WHERE tg_id = ${tgId}`;
  }

  /** Inserts a subscription with a fresh public token and its origins. */
  async createSubscription(input: NewSubscription): Promise<Subscription> {
    const token = newToken();
    return await this.sql.begin(async (tx) => {
      const rows = (await tx`INSERT INTO subscriptions (user_id, name, token)
                            VALUES (${input.userId}, ${input.name}, ${token})
                            RETURNING id, user_id, name, token, created_at`) as Row[];
      const sub = mapSubscription(rows[0]!);
      for (const origin of input.origins) {
        const orows = (await tx`INSERT INTO subscription_origins
                                 (subscription_id, origin_url, hwid, hwid_mode, hwid_param)
                               VALUES (${sub.id}, ${origin.url}, ${origin.hwid},
                                       ${origin.hwidMode}, ${origin.hwidParam})
                               RETURNING id, subscription_id, origin_url, hwid, hwid_mode, hwid_param, enabled, created_at`) as Row[];
        sub.origins.push(mapOrigin(orows[0]!));
      }
      return sub;
    });
  }

  /** Returns all subscriptions of the user, newest first, with their origins. */
  async listSubscriptions(userId: number): Promise<Subscription[]> {
    const rows = (await this.sql`SELECT id, user_id, name, token, created_at
                                 FROM subscriptions WHERE user_id = ${userId}
                                 ORDER BY id DESC`) as Row[];
    const subs = rows.map(mapSubscription);
    await this.attachOrigins(subs);
    return subs;
  }

  /** Returns one subscription owned by the user, with origins. */
  async getSubscription(userId: number, id: number): Promise<Subscription> {
    const rows = (await this.sql`SELECT id, user_id, name, token, created_at
                                 FROM subscriptions WHERE user_id = ${userId} AND id = ${id}`) as Row[];
    const row = rows[0];
    if (row === undefined) throw new NotFoundError();
    const sub = mapSubscription(row);
    await this.attachOrigins([sub]);
    return sub;
  }

  /** Returns a subscription by its public token (no ownership check). */
  async getByToken(token: string): Promise<Subscription> {
    const rows = (await this.sql`SELECT id, user_id, name, token, created_at
                                 FROM subscriptions WHERE token = ${token}`) as Row[];
    const row = rows[0];
    if (row === undefined) throw new NotFoundError();
    const sub = mapSubscription(row);
    await this.attachOrigins([sub]);
    return sub;
  }

  /** Removes a subscription owned by the user. */
  async deleteSubscription(userId: number, id: number): Promise<void> {
    const rows = (await this.sql`DELETE FROM subscriptions
                                 WHERE user_id = ${userId} AND id = ${id}
                                 RETURNING id`) as Row[];
    if (rows.length === 0) throw new NotFoundError();
  }

  /** Changes the display name of a subscription. */
  async renameSubscription(userId: number, id: number, name: string): Promise<void> {
    const rows = (await this.sql`UPDATE subscriptions SET name = ${name}, updated_at = now()
                                 WHERE user_id = ${userId} AND id = ${id}
                                 RETURNING id`) as Row[];
    if (rows.length === 0) throw new NotFoundError();
  }

  /** Attaches a new origin to a subscription owned by the user. */
  async addOrigin(userId: number, subId: number, origin: NewOrigin): Promise<Origin> {
    return await this.sql.begin(async (tx) => {
      const owned = (await tx`SELECT id FROM subscriptions
                             WHERE user_id = ${userId} AND id = ${subId}`) as Row[];
      if (owned.length === 0) throw new NotFoundError();

      const rows = (await tx`INSERT INTO subscription_origins
                              (subscription_id, origin_url, hwid, hwid_mode, hwid_param)
                            VALUES (${subId}, ${origin.url}, ${origin.hwid},
                                    ${origin.hwidMode}, ${origin.hwidParam})
                            RETURNING id, subscription_id, origin_url, hwid, hwid_mode, hwid_param, enabled, created_at`) as Row[];
      return mapOrigin(rows[0]!);
    });
  }

  /**
   * Removes an origin owned by the user and returns the id of its subscription.
   * The last origin of a subscription cannot be removed.
   */
  async deleteOrigin(userId: number, originId: number): Promise<number> {
    return await this.sql.begin(async (tx) => {
      const rows = (await tx`SELECT o.subscription_id
                            FROM subscription_origins o
                            JOIN subscriptions s ON s.id = o.subscription_id
                            WHERE o.id = ${originId} AND s.user_id = ${userId}`) as Row[];
      const row = rows[0];
      if (row === undefined) throw new NotFoundError();
      const subId = num(row.subscription_id);

      // Locking the subscription serializes concurrent deletes of its origins.
      await tx`SELECT id FROM subscriptions WHERE id = ${subId} FOR UPDATE`;
      const counts = (await tx`SELECT count(*)::int AS n FROM subscription_origins
                              WHERE subscription_id = ${subId}`) as Row[];
      if (num(counts[0]!.n) <= 1) throw new LastOriginError();

      await tx`DELETE FROM subscription_origins WHERE id = ${originId}`;
      return subId;
    });
  }

  /** Returns one origin owned by the user together with its subscription id. */
  async getOrigin(userId: number, originId: number): Promise<{ origin: Origin; subscriptionId: number }> {
    const rows = (await this.sql`
      SELECT o.id, o.subscription_id, o.origin_url, o.hwid, o.hwid_mode, o.hwid_param, o.enabled, o.created_at
      FROM subscription_origins o
      JOIN subscriptions s ON s.id = o.subscription_id
      WHERE o.id = ${originId} AND s.user_id = ${userId}`) as Row[];
    const row = rows[0];
    if (row === undefined) throw new NotFoundError();
    return { origin: mapOrigin(row), subscriptionId: num(row.subscription_id) };
  }

  /** Enables or disables an origin owned by the user. */
  async setOriginEnabled(userId: number, originId: number, enabled: boolean): Promise<void> {
    const rows = (await this.sql`
      UPDATE subscription_origins
      SET enabled = ${enabled}
      WHERE id = ${originId}
        AND subscription_id IN (SELECT id FROM subscriptions WHERE user_id = ${userId})
      RETURNING id`) as Row[];
    if (rows.length === 0) throw new NotFoundError();
  }

  /**
   * Records a subscription fetch, creating the device on first contact and
   * bumping counters and metadata afterwards. Empty metadata never overwrites
   * previously known values.
   */
  async touchDevice(subId: number, d: DeviceInfo): Promise<void> {
    await this.sql`
      INSERT INTO devices
        (subscription_id, device_key, hwid, user_agent, device_os, os_version, device_model, ip)
      VALUES (${subId}, ${d.key}, ${d.hwid}, ${d.userAgent}, ${d.os}, ${d.osVersion}, ${d.model}, ${d.ip})
      ON CONFLICT (subscription_id, device_key) DO UPDATE SET
        requests     = devices.requests + 1,
        last_seen    = now(),
        hwid         = COALESCE(NULLIF(EXCLUDED.hwid, ''), devices.hwid),
        user_agent   = COALESCE(NULLIF(EXCLUDED.user_agent, ''), devices.user_agent),
        device_os    = COALESCE(NULLIF(EXCLUDED.device_os, ''), devices.device_os),
        os_version   = COALESCE(NULLIF(EXCLUDED.os_version, ''), devices.os_version),
        device_model = COALESCE(NULLIF(EXCLUDED.device_model, ''), devices.device_model),
        ip           = EXCLUDED.ip`;
  }

  /** Returns the devices of a subscription, most recent first. */
  async listDevices(subId: number, limit: number): Promise<Device[]> {
    const effectiveLimit = limit <= 0 ? 50 : limit;
    const rows = (await this.sql`
      SELECT device_key, hwid, user_agent, device_os, os_version, device_model, ip,
             requests, first_seen, last_seen
      FROM devices
      WHERE subscription_id = ${subId}
      ORDER BY last_seen DESC
      LIMIT ${effectiveLimit}`) as Row[];
    return rows.map((row) => ({
      deviceKey: String(row.device_key),
      hwid: String(row.hwid),
      userAgent: String(row.user_agent),
      os: String(row.device_os),
      osVersion: String(row.os_version),
      model: String(row.device_model),
      ip: String(row.ip),
      requests: num(row.requests),
      firstSeen: date(row.first_seen),
      lastSeen: date(row.last_seen),
    }));
  }

  /** Loads the origins of the given subscriptions in one query. */
  private async attachOrigins(subs: Subscription[]): Promise<void> {
    if (subs.length === 0) return;
    const ids = subs.map((s) => s.id);
    const rows = (await this.sql`SELECT id, subscription_id, origin_url, hwid, hwid_mode, hwid_param, enabled, created_at
                                 FROM subscription_origins
                                 WHERE subscription_id IN ${sqlList(ids)}
                                 ORDER BY id`) as Row[];
    const byId = new Map(subs.map((s) => [s.id, s]));
    for (const row of rows) {
      const sub = byId.get(num(row.subscription_id));
      if (sub) sub.origins.push(mapOrigin(row));
    }
  }
}

/** Generates a short URL-safe public token for a subscription link. */
function newToken(): string {
  const bytes = new Uint8Array(18);
  crypto.getRandomValues(bytes);
  return Buffer.from(bytes).toString("base64url");
}
