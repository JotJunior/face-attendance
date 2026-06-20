-- Migration 004: create attendance_events table
-- Stores all face recognition webhook events for idempotency (event_key) and audit.

CREATE TABLE IF NOT EXISTS attendance_events (
    id                  BIGSERIAL       PRIMARY KEY,
    event_key           VARCHAR(128)    NOT NULL,
    employee_no_string  VARCHAR(32)     NOT NULL,
    federal_document    VARCHAR(14),
    member_id           BIGINT          REFERENCES members(id),
    device_id           BIGINT          REFERENCES devices(id),
    event_datetime      TIMESTAMPTZ,
    attendance_status   VARCHAR(32),
    marked              BOOLEAN         NOT NULL DEFAULT false,
    marked_at           TIMESTAMPTZ,
    raw_payload         JSONB           NOT NULL,
    created_at          TIMESTAMPTZ     NOT NULL DEFAULT now(),
    CONSTRAINT attendance_events_event_key_unique UNIQUE (event_key)
);

CREATE INDEX IF NOT EXISTS idx_attendance_events_federal_document ON attendance_events (federal_document);
CREATE INDEX IF NOT EXISTS idx_attendance_events_member_id ON attendance_events (member_id);
