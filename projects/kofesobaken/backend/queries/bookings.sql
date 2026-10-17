-- name: CreateBooking :one
INSERT INTO bookings (name, phone, day, at, guests, with_dog, prepay_kop, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'awaiting_payment')
RETURNING *;

-- name: GetBooking :one
SELECT * FROM bookings WHERE id = $1;

-- name: ListBookings :many
SELECT * FROM bookings WHERE status <> 'awaiting_payment' ORDER BY created_at DESC LIMIT 300;

-- name: MarkBookingPaid :one
UPDATE bookings SET status = 'new', paid_at = now() WHERE id = $1 AND status = 'awaiting_payment' RETURNING *;

-- name: SetBookingStatus :one
UPDATE bookings SET status = $2 WHERE id = $1 AND status = ANY(@from_status::text[]) RETURNING *;
