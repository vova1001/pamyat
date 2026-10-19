-- Фото из кабинета в PostgreSQL — для хостинга без постоянного диска (Render). Объём — десятки фото.
-- +goose Up
CREATE TABLE media (
    name         TEXT PRIMARY KEY,
    content_type TEXT NOT NULL,
    data         BYTEA NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE media;
