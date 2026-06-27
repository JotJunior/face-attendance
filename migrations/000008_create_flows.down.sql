-- Rollback migration 000008: remover tabelas de fluxos na ordem inversa das FKs.
DROP TABLE IF EXISTS flow_execution_logs;
DROP TABLE IF EXISTS background_images;
DROP TABLE IF EXISTS flows;
