# AGENTS.md

## What this is
One Go module (`github.com/infybtw/ProxyConnectorBot`) producing a single binary, `pcb`: a Telegram bot that binds a stable HWID to VPN subscriptions, plus a fiber HTTP proxy at `GET /s/:token` that replays the provider's response with that HWID attached. Postgres for persistence. Requires Go 1.27.1+ (see `go.mod` / `Dockerfile`).

## Commands
- Build: `go build ./cmd/pcb` — entrypoint lives in `cmd/pcb`, not the repo root.
- Run locally: `go run ./cmd/pcb`. It `godotenv.Load()`s `.env` from the working dir, needs a reachable Postgres, and runs migrations automatically at startup.
- Unit tests: `go test ./...` (no DB required).
- Single test: `go test ./internal/web/ -run TestSubscriptionPassthrough`.
- Integration test: only `internal/web/server_test.go` touches a DB. It **skips** unless `TEST_DATABASE_URL` is set, and when set it migrates that DB and creates/deletes its own rows — point it at a disposable Postgres.
- No Makefile, linter config, or task runner. Checks are `gofmt` + `go vet ./...`.
- Dev stack: `cp .env.example .env && docker compose -f docker-compose.dev.yml up --build`. Postgres is exposed on host `localhost:5433`; Caddy is on `http://localhost:8080` (see `Caddyfile.dev`).
- Prod stack: `docker compose -f docker-compose.yml up -d --build` (Caddy gets a Let's Encrypt cert for `APP_DOMAIN`).
- CI (`.github/workflows/ci.yml`) runs **only on `v*` tags** and only builds/pushes the image to GHCR — it does not test or lint. Run tests locally before tagging.

## Required env
`config.Load` fails without `TELEGRAM_BOT_TOKEN`, `DATABASE_URL`, and `PUBLIC_BASE_URL`. Compose assembles `DATABASE_URL` from `POSTGRES_*`, so it is only set manually when running outside Docker (host port 5433). All other defaults (device headers, locale, timeout) are in `internal/config/config.go` and documented in `.env.example`.

## Architecture / gotchas
- `cmd/pcb/main.go` starts fiber HTTP and bot long-polling concurrently and shuts both down on SIGINT/SIGTERM or first failure.
- Migrations are `//go:embed`ded from `internal/store/migrations/*.sql`, applied in filename sort order and tracked in `schema_migrations`. Add a new `NNNN_name.sql` file; **never edit an applied migration** — there are no checksums, so it will not re-run.
- i18n strings live in both `internal/i18n/locales/ru.json` and `en.json`. `i18n.T(lang, key, args...)` replaces `{0}`-style placeholders; unknown keys fall back to Russian then log a warning. Add every new bot string to both files. `internal/i18n` panics at init on a missing/invalid locale file.
- HWID delivery is per subscription: `header` (default; the header name is stored in `hwid_param`, normally `x-hwid`) or `query`. `detectHWIDMode` (`internal/botapp/messages.go`) picks query mode only when the origin URL already has a query key containing `hwid`, reusing that key name.
- `/s/:token` is a transparent passthrough (status, body, headers), but `internal/web/server.go` strips hop-by-hop headers plus identity headers `x-hwid`, `x-device-os`, `x-ver-os`, `x-device-model` so the bound HWID never leaks to clients. Keep `hopByHopHeaders` in sync when adding identity headers.
- Device tracking (`devices` table) groups requests by HWID when present, else by an 8-byte metadata fingerprint. `TouchDevice` uses `COALESCE(NULLIF(...))` so empty metadata never overwrites known values. Real client IP comes from `X-Forwarded-For` (Caddy).
- Bot multi-step flows are in-memory per Telegram user (`Bot.flows`, mutex-guarded) and are lost on restart; subscriptions/HWIDs are persisted.
- Telegram caps `callback_data` at 64 bytes; all callback prefixes live in `internal/botapp/views.go`.
- The Telegram wrapper is the maintainer's own module `github.com/infybtw/GoGramm` (pinned to a pseudo-version), not a common public bot library. Handlers are registered via `b.tg.Command` / `b.tg.On` in `internal/botapp/bot.go`.
- README's dev quick start points at `https://pcb.localhost`, which no longer matches `Caddyfile.dev` (a port-only `:8080` site). Trust the compose/Caddy files over the README for dev URLs.
