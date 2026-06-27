-- Reverte os FKs de devices para NO ACTION (comportamento original — RESTRICT no delete).

ALTER TABLE attendance_events DROP CONSTRAINT attendance_events_device_id_fkey;
ALTER TABLE attendance_events
  ADD CONSTRAINT attendance_events_device_id_fkey
  FOREIGN KEY (device_id) REFERENCES devices(id);

ALTER TABLE member_processing_status DROP CONSTRAINT member_processing_status_device_id_fkey;
ALTER TABLE member_processing_status
  ADD CONSTRAINT member_processing_status_device_id_fkey
  FOREIGN KEY (device_id) REFERENCES devices(id);

ALTER TABLE flow_execution_logs DROP CONSTRAINT flow_execution_logs_device_id_fkey;
ALTER TABLE flow_execution_logs
  ADD CONSTRAINT flow_execution_logs_device_id_fkey
  FOREIGN KEY (device_id) REFERENCES devices(id);
