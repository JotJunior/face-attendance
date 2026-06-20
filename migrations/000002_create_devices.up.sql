-- Migration 002: create devices table
-- Stores HikVision device registrations (device_identifier is the natural key).

CREATE TABLE IF NOT EXISTS devices (
    id                  BIGSERIAL       PRIMARY KEY,
    device_identifier   VARCHAR(64)     NOT NULL,
    ip_address          INET,
    mac_address         VARCHAR(17),
    last_heartbeat_at   TIMESTAMPTZ,
    is_active           BOOLEAN         NOT NULL DEFAULT true,
    webhook_configured  BOOLEAN         NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ     NOT NULL DEFAULT now(),
    CONSTRAINT devices_device_identifier_unique UNIQUE (device_identifier)
);
