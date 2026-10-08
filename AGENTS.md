# AGENTS.md

## What this is
A Bun + TypeScript service (no build step) producing one process, `pcb`: a
Telegram bot (grammY) that binds a stable HWID to VPN subscriptions, plus an
Elysia HTTP endpoint at `GET /s/:token` that replays the provider's response
with that HWID attached. Postgres is accessed with the built-in `bun.SQL`
(`import { SQL } from "bun"`). Requires Bun 1.4+ (see `Dockerfile`).

## Commands
- Install: `bun install` (Bun auto-loads `.env` when running).
- Run locally: `bun run src/index.ts` (or `bun run dev` for `--watch`). Needs a
  reachable Postgres and `TELEGRAM_BOT_TOKEN` / `DATABASE_URL` / `PUBLIC_BASE_URL`.
  Migrations run automatically at startup.
- Typecheck: `bun run typecheck` (`tsc --noEmit`).
- Unit tests: `bun test` (no DB required).
- Integration tests: `TEST_DATABASE_URL=postgres://... bun test`. Only
  `tests/integration.test.ts` touches a DB; it **skips** unless
  `TEST_DATABASE_URL` is set, migrates that DB and creates/deletes its own rows —
  point it at a disposable Postgres.
- Dev stack: `cp .env.example .env && docker compose -f docker-compose.dev.yml up --build`.
  Postgres is exposed on host `localhost:5433`; Caddy is on `http://localhost:8080`
  (see `Caddyfile.dev`).
- Prod stack: `docker compose -f docker-compose.yml up -d --build` (Caddy gets a
  Let's Encrypt cert for `APP_DOMAIN`).
- CI (`.github/workflows/ci.yml`) runs typecheck + tests on PRs; on `v*` tags it
  additionally **builds and pushes** the image to GHCR.

## Required env
`loadConfig` (`src/config.ts`) throws without `TELEGRAM_BOT_TOKEN`,
`DATABASE_URL` and `PUBLIC_BASE_URL`. Compose assembles `DATABASE_URL` from
`POSTGRES_*`, so it is only set manually when running outside Docker (host port
5433). All other defaults (device headers, locale, timeout, log level) are in
`src/config.ts` and documented in `.env.example`. `ORIGIN_TIMEOUT` uses a
Go-style duration string (`20s`, `1m30s`).

## Architecture / gotchas
- `src/index.ts` runs migrations, starts Elysia (`app.listen`) and the grammY bot
  concurrently, and shuts both down on SIGINT/SIGTERM.
- Migrations are hand-written `NNNN_name.sql` in `src/migrations`, applied in
  filename sort order by `src/migrate.ts` and tracked in `schema_migrations`.
  Add a new file; **never edit an applied migration** — there are no checksums,
  so it will not re-run.
- `bun.SQL` type quirks: `BIGINT`/`int8` comes back as a **string** (use the
  `num()` helper in `src/store.ts`), `TIMESTAMPTZ` as a `Date`, and a JS array
  cannot be bound with `= ANY(${arr})` — use the `sqlList(ids)` helper for `IN`
  (imported as `sql` from `"bun"`) or a manual placeholder list. Use
  `sql.begin(async (tx) => { ... })` for transactions; `.unsafe(sqlText)` runs
  raw/multi-statement SQL (used by the migration runner).
- i18n strings live in both `src/i18n/locales/ru.json` and `en.json`.
  `t(lang, key, args...)` replaces `{0}`-style placeholders; unknown keys fall
  back to Russian then log a warning. Add every new bot string to both files.
  `time.format` is a token pattern (`DD.MM.YYYY HH:mm`), not a Go layout.
- HWID delivery is per origin: `header` (default; header name in `hwid_param`,
  normally `x-hwid`) or `query`. `detectHwidMode` (`src/url.ts`) picks query mode
  only when the origin URL already has a query key containing `hwid`, reusing that
  key name.
- `/s/:token` is a transparent passthrough (status, body, headers), but
  `src/web.ts` strips hop-by-hop headers plus identity headers `x-hwid`,
  `x-device-os`, `x-ver-os`, `x-device-model` so the bound HWID never leaks to
  clients. Keep `HOP_BY_HOP_HEADERS` in sync when adding identity headers. The
  handler returns a raw `Response` (constructed from the origin result).
- Device tracking (`devices` table) groups requests by HWID when present, else by
  an 8-byte metadata fingerprint (`sha256`, first 16 hex chars). `touchDevice`
  uses `COALESCE(NULLIF(...))` so empty metadata never overwrites known values.
  Real client IP comes from `X-Forwarded-For` (Caddy).
- Bot multi-step flows are in-memory per Telegram user (`BotService.flows`,
  a `Map`) and are lost on restart; subscriptions/HWIDs are persisted.
- The bot registers commands on the grammY `Bot` and uses a **single**
  `bot.on("callback_query:data")` dispatcher (mirrors the old Go design) so all
  callback prefixes are handled in one place. Telegram caps `callback_data` at
  64 bytes; all callback prefixes live in `src/bot/views.ts` (`CB`). More
  specific prefixes (`sub:dev:`, `sub:t:`, …) must be matched before the generic
  `sub:`.
- The origin `User-Agent` and device headers are configurable; origin responses
  are capped at `ORIGIN_MAX_BODY` bytes (streamed read, not buffered blindly).
- README's dev quick start now matches `Caddyfile.dev` (`http://localhost:8080`).
