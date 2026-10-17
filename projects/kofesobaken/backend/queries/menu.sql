-- name: ListMenuItems :many
SELECT * FROM menu_items ORDER BY sort, id;

-- name: ListVisibleMenuItems :many
SELECT * FROM menu_items WHERE visible ORDER BY sort, id;

-- name: GetMenuItem :one
SELECT * FROM menu_items WHERE id = $1;

-- name: GetMenuItemsByIDs :many
SELECT * FROM menu_items WHERE id = ANY(@ids::bigint[]);

-- name: CreateMenuItem :one
INSERT INTO menu_items (name, description, price_kop, category, photo, visible, signature, sort)
VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE((SELECT max(sort) + 1 FROM menu_items), 0))
RETURNING *;

-- name: UpdateMenuItem :one
UPDATE menu_items
SET name = $2, description = $3, price_kop = $4, category = $5, photo = $6, visible = $7, signature = $8, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMenuItem :execrows
DELETE FROM menu_items WHERE id = $1;

-- name: CountMenuItems :one
SELECT count(*) FROM menu_items;
