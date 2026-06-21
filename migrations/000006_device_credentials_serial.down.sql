DROP INDEX IF EXISTS devices_serial_number_unique;

ALTER TABLE devices
    DROP COLUMN IF EXISTS serial_number,
    DROP COLUMN IF EXISTS model,
    DROP COLUMN IF EXISTS firmware_version,
    DROP COLUMN IF EXISTS isapi_username,
    DROP COLUMN IF EXISTS isapi_password_enc,
    DROP COLUMN IF EXISTS isapi_port;
