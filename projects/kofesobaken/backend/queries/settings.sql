-- name: GetSetting :one
SELECT value FROM settings WHERE key = $1;

-- name: PutSetting :exec
INSERT INTO settings (key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- name: InsertSettingIfMissing :exec
INSERT INTO settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING;

-- name: AddNotification :one
INSERT INTO notifications (channel, kind, recipient, title, body, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListNotifications :many
SELECT * FROM notifications ORDER BY created_at DESC, id DESC LIMIT $1;
