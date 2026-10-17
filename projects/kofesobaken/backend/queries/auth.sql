-- name: GetAdminByEmail :one
SELECT * FROM admins WHERE email = $1;

-- name: UpsertAdmin :one
INSERT INTO admins (email, password_hash) VALUES ($1, $2)
ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash
RETURNING *;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, admin_id, expires_at) VALUES ($1, $2, $3);

-- name: GetSession :one
SELECT s.token_hash, s.expires_at, a.id AS admin_id, a.email
FROM sessions s JOIN admins a ON a.id = s.admin_id
WHERE s.token_hash = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= now();
