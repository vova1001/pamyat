-- name: ListEvents :many
SELECT e.*,
  COALESCE((SELECT sum(s.people) FROM event_signups s
            WHERE s.event_id = e.id AND (s.status = 'confirmed' OR (s.status = 'awaiting_payment' AND s.hold_until > now()))), 0)::int AS taken
FROM events e
ORDER BY e.day, e.at, e.id;

-- name: ListPublishedEventsFrom :many
SELECT e.*,
  COALESCE((SELECT sum(s.people) FROM event_signups s
            WHERE s.event_id = e.id AND (s.status = 'confirmed' OR (s.status = 'awaiting_payment' AND s.hold_until > now()))), 0)::int AS taken
FROM events e
WHERE e.status = 'published' AND e.day >= @from_day::date
ORDER BY e.day, e.at, e.id
LIMIT 12;

-- name: GetEvent :one
SELECT * FROM events WHERE id = $1;

-- name: GetEventForUpdate :one
SELECT * FROM events WHERE id = $1 FOR UPDATE;

-- name: EventTaken :one
SELECT COALESCE(sum(people), 0)::int FROM event_signups
WHERE event_id = $1 AND (status = 'confirmed' OR (status = 'awaiting_payment' AND hold_until > now()));

-- name: CreateEvent :one
INSERT INTO events (title, kind, day, at, description, photo, seats, price_kop, dogs_allowed, signup_enabled, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdateEvent :one
UPDATE events
SET title = $2, kind = $3, day = $4, at = $5, description = $6, photo = $7, seats = $8, price_kop = $9,
    dogs_allowed = $10, signup_enabled = $11, status = $12, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteEvent :execrows
DELETE FROM events WHERE id = $1;

-- name: CreateSignup :one
INSERT INTO event_signups (event_id, name, via, contact, people, with_dog, amount_kop, status, hold_until, paid_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetSignup :one
SELECT * FROM event_signups WHERE id = $1;

-- name: ConfirmSignup :one
UPDATE event_signups SET status = 'confirmed', paid_at = now(), hold_until = NULL
WHERE id = $1 AND status = 'awaiting_payment' RETURNING *;

-- name: ListSignups :many
SELECT * FROM event_signups WHERE event_id = $1 AND status = 'confirmed' ORDER BY created_at;

-- name: CloseUnpaidSignup :exec
UPDATE event_signups SET status = 'cancelled', hold_until = NULL WHERE id = $1 AND status = 'awaiting_payment';
