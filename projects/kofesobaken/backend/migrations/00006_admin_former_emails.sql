-- Смена своей почты в «Профиле»: прежние почты человека. По ним ADMIN_EMAIL из окружения узнаёт администратора,
-- сменившего почту, — при перезапуске не создаётся второй вход на старую почту, admin-link находит его.
-- +goose Up
ALTER TABLE admins ADD COLUMN former_emails TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE admins DROP COLUMN former_emails;
