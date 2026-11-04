-- Metadatos de la aplicación (p. ej. la versión del catálogo de alimentos importada).
CREATE TABLE IF NOT EXISTS app_meta (
    key        text        PRIMARY KEY,
    value      text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
