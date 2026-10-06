-- 0001_init.sql: initial schema for users and subscriptions.

CREATE TABLE IF NOT EXISTS users (
    tg_id      BIGINT PRIMARY KEY,
    lang       TEXT NOT NULL DEFAULT 'ru',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(tg_id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    origin_url TEXT NOT NULL,
    hwid       TEXT NOT NULL,
    hwid_mode  TEXT NOT NULL DEFAULT 'header' CHECK (hwid_mode IN ('header', 'query')),
    hwid_param TEXT NOT NULL DEFAULT 'x-hwid',
    token      TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id ON subscriptions (user_id);
