-- name: ListStaff :many
SELECT * FROM admins ORDER BY (role = 'admin') DESC, active DESC, name, id;

-- name: CreateStaff :one
INSERT INTO admins (email, password_hash, name, role, must_change_password)
VALUES ($1, $2, $3, $4, TRUE)
RETURNING *;

-- name: UpdateStaff :one
UPDATE admins SET name = $2, email = $3, active = $4
WHERE id = $1
RETURNING *;

-- Новый временный пароль от администратора: при входе человек обязан сменить его.
-- name: ResetStaffPassword :one
UPDATE admins SET password_hash = $2, must_change_password = TRUE
WHERE id = $1
RETURNING *;
