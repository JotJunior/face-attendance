-- Migration 000010: permite remover um dispositivo ajustando os FKs dependentes.
-- Ref: feature remoção de dispositivo.
-- Semântica escolhida:
--   - attendance_events:        ON DELETE SET NULL  (preserva o histórico de presença, só desvincula o device)
--   - member_processing_status: ON DELETE CASCADE   (estado de provisionamento — re-derivável, some com o device)
--   - flow_execution_logs:      ON DELETE CASCADE    (logs operacionais de execução de fluxo do device)
-- (flows.device_id já é ON DELETE SET NULL desde a migration 000008 — sem mudança.)

ALTER TABLE attendance_events DROP CONSTRAINT attendance_events_device_id_fkey;
ALTER TABLE attendance_events
  ADD CONSTRAINT attendance_events_device_id_fkey
  FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE SET NULL;

ALTER TABLE member_processing_status DROP CONSTRAINT member_processing_status_device_id_fkey;
ALTER TABLE member_processing_status
  ADD CONSTRAINT member_processing_status_device_id_fkey
  FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE;

ALTER TABLE flow_execution_logs DROP CONSTRAINT flow_execution_logs_device_id_fkey;
ALTER TABLE flow_execution_logs
  ADD CONSTRAINT flow_execution_logs_device_id_fkey
  FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE;
