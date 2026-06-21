-- Migration 005 rollback: remove admin UI indexes
-- Reverte os índices criados pelo 000005_add_admin_indexes.up.sql.

DROP INDEX IF EXISTS idx_members_name_document;
DROP INDEX IF EXISTS idx_attendance_events_keyset;
