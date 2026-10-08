# ProxyConnectorBot

Telegram bot and HTTP proxy for VPN subscriptions (Happ/INCY) with HWID binding.

The service lets you use subscriptions bound to a device (HWID) in apps that
**do not send** the HWID when refreshing the subscription:

1. You add the subscription's origin link (the provider's link) to the bot.
2. The bot generates a HWID **once**, binds it to the subscription and stores it in Postgres.
3. The bot issues a link like `https://your-domain/s/<token>`.
4. When the app requests this link (for example, when you press "refresh"),
   the server goes to the provider with the stored HWID and returns the provider's
   response **as is** (passthrough) — now with the HWID attached.

## How the HWID is passed to the provider

Both clients (Happ and INCY) pass the HWID in the `x-hwid` header (together with
`x-device-os`, `x-ver-os`, `x-device-model`). Some panels (3x-ui, PasarGuard, etc.)
reject subscription requests without `x-hwid`. Happ also has "HWID links", where
the HWID is embedded in the URL's query parameter.

That is why each origin stores its delivery mode:

| Mode       | What is sent to the provider            | When it is used                             |
| ---------- | --------------------------------------- | ------------------------------------------- |
| `header`   | `x-hwid: <hwid>` header (default)       | regular Happ/INCY subscriptions             |
| `query`    | `?hwid=<hwid>` query parameter          | Happ HWID links                             |

- When adding an origin, the mode is chosen automatically: if the origin URL already
  contains a parameter with `hwid` in its name, query mode is used with that same
  name; otherwise header mode is used.
- The HWID format is an uppercase UUID (`8-4-4-4-12`), as in INCY. It is generated
  once when the origin is created and stored in the database.

Besides the HWID, stable device headers (`x-device-os`, `x-ver-os`,
`x-device-model`) and the User-Agent are sent to the provider — all of it is
configurable via environment variables.

## Stack

- [Bun](https://bun.sh) — runtime, package manager and test runner (TypeScript without a build step)
- [Elysia](https://elysiajs.com) — HTTP server (subscription endpoint)
- [grammY](https://grammy.dev) — Telegram Bot API (long polling)
- [Bun SQL](https://bun.sh/docs/runtime/sql) — built-in Postgres client (`bun.SQL`)
- Postgres — storage for users, subscriptions, HWIDs and devices
- Caddy — reverse proxy and HTTPS
- Docker Compose — dev and prod environments

## Structure

```
src/index.ts           — entry point (bot + HTTP + graceful shutdown)
src/config.ts          — configuration from the environment
src/migrate.ts         — runner for handwritten SQL migrations
src/migrations/*.sql   — migrations (NNNN_name.sql)
src/store.ts           — Postgres via bun.SQL
src/origin.ts          — requests to the provider with HWID substitution
src/merge.ts           — merging several subscriptions
src/hwid.ts            — HWID generation
src/url.ts             — URL helpers (HWID mode, default name)
src/web.ts             — Elysia: GET /s/:token, GET /healthz
src/bot/service.ts     — Telegram bot: commands, buttons, flows
src/bot/views.ts       — bot keyboards and texts
src/i18n/              — ru/en localization
tests/                 — bun test: unit + integration
Dockerfile, docker-compose*.yml, Caddyfile*  — dev/prod environment
```

## Quick start (dev)

```bash
cp .env.example .env
# fill in TELEGRAM_BOT_TOKEN, and APP_DOMAIN / PUBLIC_BASE_URL if needed

docker compose -f docker-compose.dev.yml up --build
```

The dev stack starts Postgres (`localhost:5433`), the app and Caddy on
`http://localhost:8080`. Open `http://localhost:8080/healthz`.

Running locally without Docker (Bun loads `.env` automatically; only Postgres is needed):

```bash
bun install
bun run src/index.ts
```

Migrations from `src/migrations` are applied automatically on startup.

## Production

```bash
cp .env.example .env
# TELEGRAM_BOT_TOKEN, PUBLIC_BASE_URL=https://sub.example.com,
# APP_DOMAIN=sub.example.com, CADDY_EMAIL, POSTGRES_PASSWORD

docker compose -f docker-compose.yml up -d --build
```

`APP_DOMAIN` must already point to the server: Caddy obtains a Let's Encrypt
certificate by itself and proxies traffic to the app.

## Development

| Command                          | Action                                     |
| -------------------------------- | ------------------------------------------ |
| `bun install`                    | install dependencies                       |
| `bun run dev`                    | run with auto-reload (`--watch`)           |
| `bun run typecheck`              | type check (`tsc --noEmit`)                |
| `bun test`                       | unit tests                                 |
| `TEST_DATABASE_URL=... bun test` | unit + integration tests                   |

Integration tests (`tests/integration.test.ts`) are **skipped** without
`TEST_DATABASE_URL`. If the variable is set, they migrate the specified database and
create/delete their own rows — use a disposable Postgres.

## Release (GitHub Actions → GHCR)

CI runs on pull requests (check: typecheck + tests) and on tags starting with `v`
(build and publish the image):

1. **check** — `bun run typecheck` and `bun test`.
2. **build** — builds the Docker image and saves it as an artifact (tags only).
3. **push** — pushes the image to GHCR: the version tag (`v1.0.0`) first, then `latest`.

```bash
git tag v1.0.0
git push origin v1.0.0
```

After the run, the image is available as:

```
ghcr.io/<owner>/<repo>:v1.0.0
ghcr.io/<owner>/<repo>:latest
```

## Environment variables

| Variable             | Required    | Default                    | Description                                          |
| -------------------- | ----------- | -------------------------- | ---------------------------------------------------- |
| `TELEGRAM_BOT_TOKEN` | yes         | —                          | bot token                                            |
| `DATABASE_URL`       | yes*        | —                          | Postgres DSN (*set automatically by compose)         |
| `PUBLIC_BASE_URL`    | yes         | —                          | public base URL for links (no trailing `/`)          |
| `APP_DOMAIN`         | yes (compose)| —                         | domain for Caddy                                     |
| `CADDY_EMAIL`        | in prod     | —                          | email for Let's Encrypt                              |
| `HTTP_ADDR`          | no          | `:8080`                    | HTTP server address                                  |
| `DEFAULT_LOCALE`     | no          | `ru`                       | default language for new users (`ru` / `en`)         |
| `HWID_DEVICE_OS`     | no          | `android`                  | value of `x-device-os` sent to the provider          |
| `HWID_VER_OS`        | no          | `14`                       | value of `x-ver-os`                                  |
| `HWID_DEVICE_MODEL`  | no          | `Pixel 7`                  | value of `x-device-model`                            |
| `ORIGIN_USER_AGENT`  | no          | `Happ/2.4.1 (Android 14)`  | User-Agent of requests to the provider               |
| `ORIGIN_TIMEOUT`     | no          | `20s`                      | timeout for provider requests (Go format: `20s`)     |
| `ORIGIN_MAX_BODY`    | no          | `20971520`                 | maximum size of the subscription body, in bytes      |
| `LOG_LEVEL`          | no          | `info`                     | log level (`debug`/`info`/`warn`/`error`)            |

## Bot

| Command   | Action                                          |
| --------- | ----------------------------------------------- |
| `/start`  | greeting and menu                               |
| `/add`    | add a subscription (URL → name)                 |
| `/subs`   | list of subscriptions                           |
| `/lang`   | interface language (русский / english)          |
| `/help`   | how everything works                            |
| `/cancel` | cancel the current flow                         |

The subscription card provides: a provider connectivity check, the device list,
renaming and deletion.

Each user sees only their own subscriptions.

### Multiple origins

One subscription can combine several sources (provider links). Each source has
its own HWID, which is bound when it is added. The "🌍 Origins" button in the
subscription card shows the list; tapping an origin opens its settings —
**enable/disable** and delete. A disabled origin is not used in `/s/:token`
responses or in checks, but stays in the database. The last origin cannot be deleted.

The HWID is never shown in the bot interface: only the origin URL, the HWID
delivery mode (header/parameter) and the status are visible.

When `/s/:token` is requested, the service queries all origins in parallel, each
with its own HWID, and merges the link lists (duplicates are removed). The response
format (plain or base64) follows the providers' responses; `subscription-userinfo`
is summed by traffic, and the nearest expiry date is used. If some origins do not
respond, the rest are returned; if none respond, the service returns 502. A
subscription with a single origin is returned as before, unchanged.

### Devices

For each `/s/:token` request, the service remembers which device made it:

- if the client sent a HWID (the `x-hwid` header or `?hwid=`) — devices are grouped
  by it and the HWID is shown;
- if there is no HWID — devices are grouped by a metadata fingerprint (User-Agent,
  `x-device-os`, `x-ver-os`, `x-device-model`) and whatever is visible is shown.

For each device, the following is stored: HWID (if any), model, OS, User-Agent, IP
(from `X-Forwarded-For`, i.e. the real client IP behind Caddy), request count and
the time of the last connection. The list is available via the "📱 Devices" button
in the subscription card (up to the 30 most recent, newest first).

## HTTP API

| Method | Path          | Description                                                     |
| ------ | ------------- | --------------------------------------------------------------- |
| GET    | `/s/:token`   | returns the provider's subscription content with the HWID set   |
| GET    | `/healthz`    | health check (`ok` / `db unavailable`)                          |

The `/s/:token` response is a transparent passthrough: the provider's status, body
and headers (`content-type`, `content-disposition`, `profile-*`,
`subscription-*`, etc.) without hop-by-hop headers. Client metadata is recorded
in the `devices` table at the same time.

## Notes

- Flow states (waiting for a URL, a name, etc.) live in process memory and are
  reset on restart — subscription data is not lost.
- The HWID is generated once when an origin is created and never changes; it is
  not displayed in the bot.
- The service does not rewrite subscription content: the provider receives the same
  HWID on every refresh, and the app receives the provider's original response.
- Migrations are regular `NNNN_name.sql` files in `src/migrations`. Applied migrations
  are recorded in `schema_migrations`; do not edit an already applied migration —
  there are no checksums, so it will not run again.
