-- 0004_origins.sql: a subscription aggregates one or more origins. Every origin
-- is a provider URL with its own HWID, delivery mode and parameter.

CREATE TABLE IF NOT EXISTS subscription_origins (
    id              BIGSERIAL PRIMARY KEY,
    subscription_id BIGINT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    origin_url      TEXT NOT NULL,
    hwid            TEXT NOT NULL,
    hwid_mode       TEXT NOT NULL DEFAULT 'header' CHECK (hwid_mode IN ('header', 'query')),
    hwid_param      TEXT NOT NULL DEFAULT 'x-hwid',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_subscription_origins_subscription_id
    ON subscription_origins (subscription_id);

INSERT INTO subscription_origins (subscription_id, origin_url, hwid, hwid_mode, hwid_param, created_at)
SELECT id, origin_url, hwid, hwid_mode, hwid_param, created_at FROM subscriptions;

ALTER TABLE subscriptions
    DROP COLUMN origin_url,
    DROP COLUMN hwid,
    DROP COLUMN hwid_mode,
    DROP COLUMN hwid_param;
