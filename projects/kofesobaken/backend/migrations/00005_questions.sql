-- Вопросы гостей: форма на сайте и сообщения Telegram-боту; ответы владелицы.
-- +goose Up
CREATE TABLE questions (
    id               BIGSERIAL PRIMARY KEY,
    source           TEXT NOT NULL CHECK (source IN ('site', 'telegram')),
    name             TEXT NOT NULL,
    contact_kind     TEXT NOT NULL CHECK (contact_kind IN ('email', 'telegram')),
    contact          TEXT NOT NULL DEFAULT '',  -- почта или @ник; у вопроса из бота — @ник, если есть
    body             TEXT NOT NULL,
    tg_chat_id       BIGINT,                    -- чат гостя с ботом (вопрос из Telegram)
    tg_message_id    BIGINT,                    -- его сообщение — ответ уходит реплаем на него
    owner_message_id BIGINT,                    -- сообщение о вопросе в чате владелицы — её реплай = ответ
    answer           TEXT NOT NULL DEFAULT '',
    answered_via     TEXT NOT NULL DEFAULT '',  -- telegram | email | manual
    answered_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX questions_created_idx ON questions (created_at DESC);
-- повторно полученное из getUpdates сообщение не создаёт второй вопрос
CREATE UNIQUE INDEX questions_tg_msg_idx ON questions (tg_chat_id, tg_message_id) WHERE tg_chat_id IS NOT NULL;
CREATE INDEX questions_owner_msg_idx ON questions (owner_message_id) WHERE owner_message_id IS NOT NULL;

-- +goose Down
DROP TABLE questions;
