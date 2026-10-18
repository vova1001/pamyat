-- name: GetAdminByEmail :one
SELECT * FROM admins WHERE email = $1;

-- name: GetAdminByID :one
SELECT * FROM admins WHERE id = $1;

-- name: GetAdminBySetupToken :one
SELECT * FROM admins WHERE setup_token_hash = $1 AND setup_expires_at > now();

-- Администратор из окружения: создаётся, если его нет; пароль меняется, только если передан
-- (временный — с обязательной сменой при первом входе).
-- name: EnsureAdmin :one
INSERT INTO admins (email, password_hash, role, name, must_change_password) VALUES ($1, $2, 'admin', 'Администратор', $3)
ON CONFLICT (email) DO UPDATE SET
    role = 'admin', active = TRUE,
    must_change_password = CASE WHEN EXCLUDED.password_hash <> '' THEN EXCLUDED.must_change_password ELSE admins.must_change_password END,
    password_hash = CASE WHEN EXCLUDED.password_hash <> '' THEN EXCLUDED.password_hash ELSE admins.password_hash END
RETURNING *;

-- name: SetSetupToken :exec
UPDATE admins SET setup_token_hash = $2, setup_expires_at = $3 WHERE id = $1;

-- Пароль задан самим человеком: временный пароль и ссылка настройки больше не действуют.
-- name: SetOwnPassword :exec
UPDATE admins SET password_hash = $2, must_change_password = FALSE, setup_token_hash = NULL, setup_expires_at = NULL
WHERE id = $1;

-- name: TouchLogin :exec
UPDATE admins SET last_login_at = now() WHERE id = $1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, admin_id, expires_at) VALUES ($1, $2, $3);

-- name: GetSession :one
SELECT s.token_hash, s.expires_at, a.id AS admin_id, a.email, a.name, a.role, a.must_change_password
FROM sessions s JOIN admins a ON a.id = s.admin_id
WHERE s.token_hash = $1 AND s.expires_at > now() AND a.active;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteAdminSessions :exec
DELETE FROM sessions WHERE admin_id = $1;

-- name: DeleteOtherSessions :exec
DELETE FROM sessions WHERE admin_id = $1 AND token_hash <> $2;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();

-- Своя почта из «Профиля»: прежняя запоминается (см. GetAdminByFormerEmail).
-- name: SetOwnEmail :one
UPDATE admins SET email = $2, former_emails = array_append(array_remove(former_emails, email), email)
WHERE id = $1
RETURNING *;

-- Администратор, у которого эта почта была раньше (сменил в «Профиле»); самый свежий — первым.
-- name: GetAdminByFormerEmail :one
SELECT * FROM admins WHERE $1::text = ANY(former_emails) ORDER BY id DESC LIMIT 1;
