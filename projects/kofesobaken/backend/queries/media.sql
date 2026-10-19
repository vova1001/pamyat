-- name: SaveMedia :exec
INSERT INTO media (name, content_type, data) VALUES ($1, $2, $3);

-- name: GetMedia :one
SELECT content_type, data, created_at FROM media WHERE name = $1;
