-- URL Shortener Schema
-- Optimized for high read throughput, unique constraints, and partitioned/indexed click telemetry.

CREATE TABLE IF NOT EXISTS urls (
    id BIGINT PRIMARY KEY,
    short_code VARCHAR(32) NOT NULL,
    original_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NULL,
    is_custom BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT uq_urls_short_code UNIQUE (short_code)
);

CREATE INDEX IF NOT EXISTS idx_urls_short_code ON urls (short_code);
CREATE INDEX IF NOT EXISTS idx_urls_expires_at ON urls (expires_at) WHERE expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS click_events (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(32) NOT NULL,
    clicked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip_address VARCHAR(64),
    user_agent TEXT,
    referrer TEXT
);

CREATE INDEX IF NOT EXISTS idx_click_events_short_code_time ON click_events (short_code, clicked_at DESC);
