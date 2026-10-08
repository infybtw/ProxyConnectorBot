// Minimal structured logger with slog-like key/value output.

type Level = "debug" | "info" | "warn" | "error";

const LEVELS: Record<Level, number> = { debug: 0, info: 1, warn: 2, error: 3 };

const minLevel: Level = (() => {
  const raw = (Bun.env["LOG_LEVEL"] ?? "info").toLowerCase();
  return (raw in LEVELS ? raw : "info") as Level;
})();

/** Extra structured fields attached to a log line. */
export type Fields = Record<string, unknown>;

function write(level: Level, msg: string, fields?: Fields): void {
  if (LEVELS[level] < LEVELS[minLevel]) return;
  const time = new Date().toISOString();
  let line = `time=${time} level=${level} msg=${JSON.stringify(msg)}`;
  if (fields) {
    for (const [key, value] of Object.entries(fields)) {
      line += ` ${key}=${formatValue(value)}`;
    }
  }
  const stream = level === "error" || level === "warn" ? console.error : console.log;
  stream(line);
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined) return "null";
  if (value instanceof Error) return JSON.stringify(value.message);
  if (typeof value === "string") return JSON.stringify(value);
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

/** Structured logger. */
export const log = {
  debug: (msg: string, fields?: Fields): void => write("debug", msg, fields),
  info: (msg: string, fields?: Fields): void => write("info", msg, fields),
  warn: (msg: string, fields?: Fields): void => write("warn", msg, fields),
  error: (msg: string, fields?: Fields): void => write("error", msg, fields),
};
