-- Migration 003: create member_processing_status table
-- Tracks per-member per-device ISAPI processing state (idempotency key: federal_document + device_id).

CREATE TABLE IF NOT EXISTS member_processing_status (
    id                  BIGSERIAL       PRIMARY KEY,
    federal_document    VARCHAR(14)     NOT NULL,
    device_id           BIGINT          NOT NULL REFERENCES devices(id),
    user_synced         BOOLEAN         NOT NULL DEFAULT false,
    face_uploaded       BOOLEAN         NOT NULL DEFAULT false,
    webhook_set         BOOLEAN         NOT NULL DEFAULT false,
    last_stage          VARCHAR(32),
    last_error          TEXT,
    attempts            INT             NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ     NOT NULL DEFAULT now(),
    CONSTRAINT member_processing_status_federal_document_device_id_unique
        UNIQUE (federal_document, device_id)
);
