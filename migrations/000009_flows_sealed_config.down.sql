-- Rollback migration 000009: remover coluna sealed_config de flows.
ALTER TABLE flows DROP COLUMN IF EXISTS sealed_config;
