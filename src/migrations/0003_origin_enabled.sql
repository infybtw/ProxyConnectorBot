-- 0003_origin_enabled.sql: per-origin on/off switch. A disabled origin is kept
-- in the database but excluded from subscription serving and origin checks.

ALTER TABLE subscription_origins
    ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT true;
