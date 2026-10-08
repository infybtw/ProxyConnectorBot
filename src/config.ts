// Application configuration loaded from the environment.

/** Device identity sent to origin servers together with the HWID. */
export interface DeviceIdentity {
  /** Value of the x-device-os header. */
  os: string;
  /** Value of the x-ver-os header. */
  osVersion: string;
  /** Value of the x-device-model header. */
  model: string;
  /** User-Agent sent to origin servers. */
  userAgent: string;
}

/** All runtime settings of the application. */
export interface Config {
  /** Token of the Telegram bot from @BotFather. */
  telegramBotToken: string;
  /** Postgres connection string. */
  databaseUrl: string;
  /** Public base URL used to build subscription links, without a trailing slash. */
  publicBaseUrl: string;
  /** Listen address of the HTTP server, e.g. ":8080" or "0.0.0.0:8080". */
  httpAddr: string;
  /** Locale assigned to users without a saved choice. */
  defaultLocale: string;
  /** Fake device identity sent to origin servers. */
  device: DeviceIdentity;
  /** Timeout of a single origin request, in milliseconds. */
  originTimeoutMs: number;
  /** Maximum size of an origin response body, in bytes. */
  originMaxBody: number;
}

/** Parsed listen address. */
export interface ListenAddr {
  hostname: string;
  port: number;
}

function getenv(key: string, fallback: string): string {
  const value = Bun.env[key];
  return value === undefined || value === "" ? fallback : value;
}

/**
 * parseDuration parses a Go-style duration string such as "20s", "1m30s" or
 * "500ms" into milliseconds. A bare number is treated as seconds.
 */
export function parseDurationMs(value: string): number {
  const input = value.trim();
  if (input === "") throw new Error("empty duration");
  if (/^\d+(\.\d+)?$/.test(input)) return Number(input) * 1000;

  const units: Record<string, number> = {
    ns: 1e-6,
    us: 1e-3,
    "µs": 1e-3,
    ms: 1,
    s: 1000,
    m: 60_000,
    h: 3_600_000,
  };

  const re = /(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g;
  let total = 0;
  let matchedLength = 0;
  let match: RegExpExecArray | null;
  while ((match = re.exec(input)) !== null) {
    matchedLength += match[0].length;
    total += Number(match[1]) * units[match[2]!]!;
  }
  if (matchedLength === 0 || matchedLength !== input.length) {
    throw new Error(`invalid duration: ${value}`);
  }
  return total;
}

/** Parses ":8080" / "0.0.0.0:8080" / "8080" into hostname and port. */
export function parseListenAddr(addr: string): ListenAddr {
  const trimmed = addr.trim();
  const lastColon = trimmed.lastIndexOf(":");
  if (lastColon === -1) {
    return { hostname: "0.0.0.0", port: Number(trimmed) };
  }
  const hostname = trimmed.slice(0, lastColon) || "0.0.0.0";
  const port = Number(trimmed.slice(lastColon + 1));
  return { hostname, port };
}

/** Loads configuration from the environment and validates required values. */
export function loadConfig(env: Record<string, string | undefined> = Bun.env): Config {
  const read = (key: string, fallback: string): string => {
    const value = env[key];
    return value === undefined || value === "" ? fallback : value;
  };

  const timeoutRaw = env["ORIGIN_TIMEOUT"];
  let originTimeoutMs = 20_000;
  if (timeoutRaw) {
    originTimeoutMs = parseDurationMs(timeoutRaw);
  }

  const maxBodyRaw = env["ORIGIN_MAX_BODY"];
  let originMaxBody = 20 << 20;
  if (maxBodyRaw) {
    originMaxBody = Number(maxBodyRaw);
    if (!Number.isFinite(originMaxBody) || originMaxBody <= 0) {
      throw new Error(`ORIGIN_MAX_BODY: invalid number: ${maxBodyRaw}`);
    }
  }

  const config: Config = {
    telegramBotToken: env["TELEGRAM_BOT_TOKEN"] ?? "",
    databaseUrl: env["DATABASE_URL"] ?? "",
    publicBaseUrl: (env["PUBLIC_BASE_URL"] ?? "").replace(/\/+$/, ""),
    httpAddr: read("HTTP_ADDR", ":8080"),
    defaultLocale: read("DEFAULT_LOCALE", "ru"),
    device: {
      os: read("HWID_DEVICE_OS", "android"),
      osVersion: read("HWID_VER_OS", "14"),
      model: read("HWID_DEVICE_MODEL", "Pixel 7"),
      userAgent: read("ORIGIN_USER_AGENT", "Happ/2.4.1 (Android 14)"),
    },
    originTimeoutMs,
    originMaxBody,
  };

  const missing: string[] = [];
  if (config.telegramBotToken === "") missing.push("TELEGRAM_BOT_TOKEN");
  if (config.databaseUrl === "") missing.push("DATABASE_URL");
  if (config.publicBaseUrl === "") missing.push("PUBLIC_BASE_URL");
  if (missing.length > 0) {
    throw new Error(`missing required environment variables: ${missing.join(", ")}`);
  }

  return config;
}
