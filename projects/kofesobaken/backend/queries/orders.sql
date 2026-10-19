-- name: CreateOrder :one
INSERT INTO orders (mode, phone, address, total_kop, delivery_kop, status)
VALUES ($1, $2, $3, $4, $5, 'awaiting_payment')
RETURNING *;

-- name: AddOrderItem :exec
INSERT INTO order_items (order_id, menu_item_id, name, price_kop, qty) VALUES ($1, $2, $3, $4, $5);

-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1;

-- name: ListOrders :many
SELECT * FROM orders WHERE NOT (status = 'cancelled' AND paid_at IS NULL) ORDER BY created_at DESC LIMIT 200;

-- name: ListOrderItems :many
SELECT * FROM order_items WHERE order_id = ANY(@ids::bigint[]) ORDER BY id;

-- name: MarkOrderPaid :one
UPDATE orders SET status = 'new', paid_at = now() WHERE id = $1 AND status = 'awaiting_payment' RETURNING *;

-- name: SetOrderStatus :one
UPDATE orders SET status = $2 WHERE id = $1 AND status = 'new' RETURNING *;

-- name: CloseUnpaidOrder :exec
UPDATE orders SET status = 'cancelled' WHERE id = $1 AND status = 'awaiting_payment';
