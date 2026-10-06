-- 0002_devices.sql: devices that fetched a subscription through our domain.

CREATE TABLE IF NOT EXISTS devices (
    id              BIGSERIAL PRIMARY KEY,
    subscription_id BIGINT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    -- device_key groups requests from the same device: HWID when the client
    -- sends one, otherwise a fingerprint of its metadata (UA/OS/model).
    device_key      TEXT NOT NULL,
    hwid            TEXT NOT NULL DEFAULT '',
    user_agent      TEXT NOT NULL DEFAULT '',
    device_os       TEXT NOT NULL DEFAULT '',
    os_version      TEXT NOT NULL DEFAULT '',
    device_model    TEXT NOT NULL DEFAULT '',
    ip              TEXT NOT NULL DEFAULT '',
    requests        BIGINT NOT NULL DEFAULT 1,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (subscription_id, device_key)
);

CREATE INDEX IF NOT EXISTS idx_devices_subscription_last_seen
    ON devices (subscription_id, last_seen DESC);
