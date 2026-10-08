// Command pcb (TypeScript) runs the ProxyConnectorBot: a Telegram bot managing
// HWID-bound VPN subscriptions plus an HTTP endpoint serving them on our own
// domain. Bun loads .env automatically.

import { SQL } from "bun";
import { BotService } from "./bot/service";
import { loadConfig, parseListenAddr } from "./config";
import { log } from "./log";
import { migrate } from "./migrate";
import { OriginClient } from "./origin";
import { Store } from "./store";
import { createServer } from "./web";

async function main(): Promise<void> {
  let cfg;
  try {
    cfg = loadConfig();
  } catch (err) {
    log.error("config error", { err });
    process.exit(1);
  }

  const address = parseListenAddr(cfg.httpAddr);
  if (!Number.isInteger(address.port) || address.port <= 0) {
    log.error("config error", { err: `invalid HTTP_ADDR: ${cfg.httpAddr}` });
    process.exit(1);
  }

  const sql = new SQL(cfg.databaseUrl);
  try {
    await migrate(sql);
  } catch (err) {
    log.error("db: migrate failed", { err });
    await sql.close();
    process.exit(1);
  }

  const store = new Store(sql);
  const origin = new OriginClient(cfg.originTimeoutMs, cfg.originMaxBody, cfg.device);
  const app = createServer(store, origin);

  try {
    app.listen({ hostname: address.hostname, port: address.port });
  } catch (err) {
    log.error("http: listen failed", { addr: cfg.httpAddr, err });
    await sql.close();
    process.exit(1);
  }
  log.info("http: listening", { addr: cfg.httpAddr });

  const bot = new BotService(store, origin, cfg);

  let shuttingDown = false;
  const shutdown = async (signal: string): Promise<void> => {
    if (shuttingDown) return;
    shuttingDown = true;
    log.info("shutdown requested", { signal });
    bot.stop();
    try {
      await app.stop();
    } catch (err) {
      log.error("http: shutdown failed", { err });
    }
    await sql.close();
    log.info("bye");
    process.exit(0);
  };
  process.on("SIGINT", () => void shutdown("SIGINT"));
  process.on("SIGTERM", () => void shutdown("SIGTERM"));

  try {
    await bot.run();
  } catch (err) {
    if (!shuttingDown) {
      log.error("bot: polling failed", { err });
      process.exit(1);
    }
  }
}

await main();
