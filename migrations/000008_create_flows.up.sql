-- Migration 000008: criar tabelas de fluxos para a feature face-flow.
-- Ref: docs/specs/face-flow/data-model.md §Migration

-- Tabela principal de fluxos configurados pelo admin.
CREATE TABLE flows (
    id         BIGSERIAL    PRIMARY KEY,
    name       TEXT         NOT NULL,
    status     TEXT         NOT NULL DEFAULT 'inactive'
                            CHECK (status IN ('active', 'inactive')),
    device_id  BIGINT       UNIQUE REFERENCES devices(id) ON DELETE SET NULL,
    nodes      JSONB        NOT NULL DEFAULT '[]',
    edges      JSONB        NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_flows_status    ON flows(status);
CREATE INDEX idx_flows_device_id ON flows(device_id) WHERE device_id IS NOT NULL;

-- Biblioteca de imagens de fundo disponíveis para seleção no nó change_background.
CREATE TABLE background_images (
    id         BIGSERIAL    PRIMARY KEY,
    name       TEXT         NOT NULL,
    file_path  TEXT         NOT NULL,   -- path relativo a BACKGROUND_IMAGES_DIR
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Registro de cada execução do motor de fluxo.
-- Idempotência garantida por event_key UNIQUE (Constituição §II, spec FR-023).
CREATE TABLE flow_execution_logs (
    id             BIGSERIAL    PRIMARY KEY,
    flow_id        BIGINT       NOT NULL REFERENCES flows(id),
    device_id      BIGINT       NOT NULL REFERENCES devices(id),
    event_key      TEXT         NOT NULL,
    status         TEXT         NOT NULL
                                CHECK (status IN ('completed', 'circuit_break')),
    failed_node_id TEXT,                -- node.id do nó que falhou (NULL se completed)
    error          TEXT,                -- mensagem de erro (NULL se completed)
    started_at     TIMESTAMPTZ  NOT NULL,
    finished_at    TIMESTAMPTZ  NOT NULL,
    CONSTRAINT uq_flow_execution_logs_event_key UNIQUE (event_key)
);

CREATE INDEX idx_flow_execution_logs_flow_id   ON flow_execution_logs(flow_id);
CREATE INDEX idx_flow_execution_logs_device_id ON flow_execution_logs(device_id);
CREATE INDEX idx_flow_execution_logs_started_at ON flow_execution_logs(started_at DESC);
