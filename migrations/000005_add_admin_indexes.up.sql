-- Migration 005: add admin UI indexes
-- Suporta queries de busca e paginação keyset da interface de administração.
-- Todos os índices usam IF NOT EXISTS para idempotência em ambientes de dev/test.

-- Índice composto em members para busca icase por nome e CPF (query q= da tela de membros)
-- Ref: tasks.md 1.3.2, CHK-P11, plan.md §ListMembersPaged
CREATE INDEX IF NOT EXISTS idx_members_name_document
    ON members (name, federal_document);

-- Índice composto em attendance_events para paginação keyset (created_at DESC, id DESC)
-- Suporta ListEventsPaged sem full scan; a ordem DESC espelha a ordenação da query.
-- Ref: tasks.md 1.3.3, CHK-P11, plan.md §ListEventsPaged
CREATE INDEX IF NOT EXISTS idx_attendance_events_keyset
    ON attendance_events (created_at DESC, id DESC);
