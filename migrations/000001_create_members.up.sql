-- Migration 001: create members table
-- Stores GOB member data (CPF is the natural key for idempotency).

CREATE TABLE IF NOT EXISTS members (
    id              BIGSERIAL       PRIMARY KEY,
    gob_id          BIGINT          NOT NULL,
    federal_document VARCHAR(14)    NOT NULL,
    name            TEXT            NOT NULL,
    status          VARCHAR(32)     NOT NULL,
    mobile_number   VARCHAR(32),
    url_selfie      TEXT,
    gob_created_at  TIMESTAMPTZ,
    gob_updated_at  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    CONSTRAINT members_federal_document_unique UNIQUE (federal_document)
);

CREATE INDEX IF NOT EXISTS idx_members_gob_id ON members (gob_id);
