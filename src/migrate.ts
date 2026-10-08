// Applies hand-written SQL migrations in filename order.

import type { SQL } from "bun";
import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { log } from "./log";

const MIGRATIONS_DIR = path.join(import.meta.dir, "migrations");

/**
 * Applies every not-yet-applied migration from src/migrations in filename sort
 * order, tracking them in the schema_migrations table. Never edit an applied
 * migration: there are no checksums, so it will not re-run.
 */
export async function migrate(sql: SQL): Promise<void> {
  await sql.unsafe(
    `CREATE TABLE IF NOT EXISTS schema_migrations (
       name       TEXT PRIMARY KEY,
       applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
     )`,
  );

  let entries: string[];
  try {
    entries = await readdir(MIGRATIONS_DIR);
  } catch (err) {
    throw new Error(`migrate: read migrations dir: ${String(err)}`);
  }

  const names = entries.filter((name) => name.endsWith(".sql")).sort();

  for (const name of names) {
    const applied = await sql`SELECT 1 FROM schema_migrations WHERE name = ${name}`;
    if (applied.length > 0) continue;

    const raw = await readFile(path.join(MIGRATIONS_DIR, name), "utf8");
    await sql.begin(async (tx) => {
      await tx.unsafe(raw);
      await tx`INSERT INTO schema_migrations (name) VALUES (${name})`;
    });
    log.info("store: migration applied", { name });
  }
}
