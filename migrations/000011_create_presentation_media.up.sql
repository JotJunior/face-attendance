-- presentation_media mapeia cada material (imagem de presentation/start-page) de um
-- device ao seu show_mode, derivado do tamanho no momento do upload:
--   full  = 600x1024  ·  split = 600x704
-- A lista de materiais do device (ISAPI MaterialMgr) NÃO retorna as dimensões; por
-- isso persistimos o modo aqui, para reaplicar o show_mode ao selecionar/usar a
-- imagem como presentation (ex.: nó change_background de um fluxo).
CREATE TABLE IF NOT EXISTS presentation_media (
    device_id   BIGINT      NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    material_id TEXT        NOT NULL,
    mode        TEXT        NOT NULL CHECK (mode IN ('full', 'split')),
    name        TEXT        NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, material_id)
);
