-- 0001_init.sql: users, subscriptions and their origins.

CREATE TABLE users (
    tg_id      BIGINT PRIMARY KEY,
    lang       TEXT NOT NULL DEFAULT 'ru',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE subscriptions (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(tg_id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    token      TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_subscriptions_user_id ON subscriptions (user_id);

-- A subscription aggregates one or more origins. Every origin is a provider
-- URL with its own HWID, delivery mode and parameter.
CREATE TABLE subscription_origins (
    id              BIGSERIAL PRIMARY KEY,
    subscription_id BIGINT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    origin_url      TEXT NOT NULL,
    hwid            TEXT NOT NULL,
    hwid_mode       TEXT NOT NULL DEFAULT 'header' CHECK (hwid_mode IN ('header', 'query')),
    hwid_param      TEXT NOT NULL DEFAULT 'x-hwid',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_subscription_origins_subscription_id
    ON subscription_origins (subscription_id);
