-- name: CreatePayment :one
INSERT INTO payments (kind, target_id, amount_kop, description, provider, status)
VALUES ($1, $2, $3, $4, $5, 'pending')
RETURNING *;

-- name: SetPaymentProviderID :exec
UPDATE payments SET provider_payment_id = $2, updated_at = now() WHERE id = $1;

-- name: GetPayment :one
SELECT * FROM payments WHERE id = $1;

-- name: GetPaymentForUpdate :one
SELECT * FROM payments WHERE id = $1 FOR UPDATE;

-- name: GetPaymentByTarget :one
SELECT * FROM payments WHERE kind = $1 AND target_id = $2 ORDER BY created_at DESC LIMIT 1;

-- name: SetPaymentStatus :exec
UPDATE payments SET status = $2, updated_at = now() WHERE id = $1;

-- name: ListPendingPayments :many
-- Платежи, ждущие оплаты, созданные в промежутке [after, before) — для фоновой проверки статуса.
SELECT * FROM payments
WHERE status = 'pending' AND created_at >= @after AND created_at < @before
ORDER BY created_at
LIMIT 200;
