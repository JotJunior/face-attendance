-- Device serial/model/firmware (de ISAPI deviceInfo) + credenciais ISAPI no banco.
-- Move a configuração de device do .env (ISAPI_DEVICE_{N}_*) para a tabela devices:
-- a conexão passa a usar o IP corrente (atualizado pelo heartbeat) e as credenciais
-- cifradas (AES-GCM, chave mestra em ISAPI_CRED_KEY). Assim, se o device troca de IP,
-- nunca se perde a conexão. Ref: pedido do operador 2026-06-21.

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS serial_number      VARCHAR(64),
    ADD COLUMN IF NOT EXISTS model              VARCHAR(64),
    ADD COLUMN IF NOT EXISTS firmware_version   VARCHAR(32),
    ADD COLUMN IF NOT EXISTS isapi_username     VARCHAR(64),
    ADD COLUMN IF NOT EXISTS isapi_password_enc BYTEA,
    ADD COLUMN IF NOT EXISTS isapi_port         INTEGER NOT NULL DEFAULT 80;

-- Serial é a identidade de hardware (vinda via ISAPI); único quando presente.
CREATE UNIQUE INDEX IF NOT EXISTS devices_serial_number_unique
    ON devices (serial_number)
    WHERE serial_number IS NOT NULL;
