-- Кабинет для администратора и сотрудников: роли, отключение, обязательная смена временного пароля,
-- одноразовая ссылка первого входа администратора.
-- +goose Up
ALTER TABLE admins
    ADD COLUMN name                 TEXT NOT NULL DEFAULT '',
    ADD COLUMN role                 TEXT NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'staff')),
    ADD COLUMN active               BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN setup_token_hash     TEXT,
    ADD COLUMN setup_expires_at     TIMESTAMPTZ,
    ADD COLUMN last_login_at        TIMESTAMPTZ;
-- Уже существующие записи — владелец (администратор); новые по умолчанию — сотрудники.
ALTER TABLE admins ALTER COLUMN role SET DEFAULT 'staff';
UPDATE admins SET name = 'Администратор' WHERE name = '';
CREATE UNIQUE INDEX admins_setup_token_idx ON admins (setup_token_hash) WHERE setup_token_hash IS NOT NULL;
CREATE INDEX sessions_admin_idx ON sessions (admin_id);

-- +goose Down
DROP INDEX sessions_admin_idx;
DELETE FROM admins WHERE role = 'staff';
ALTER TABLE admins
    DROP COLUMN last_login_at,
    DROP COLUMN setup_expires_at,
    DROP COLUMN setup_token_hash,
    DROP COLUMN must_change_password,
    DROP COLUMN active,
    DROP COLUMN role,
    DROP COLUMN name;
