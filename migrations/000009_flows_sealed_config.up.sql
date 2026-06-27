-- Migration 000009: adicionar coluna sealed_config em flows para armazenar
-- segredos cifrados de headers de nó HTTPS (ref: tasks.md §1.1.4, spec CHK005/CHK006).
ALTER TABLE flows ADD COLUMN sealed_config JSONB;
