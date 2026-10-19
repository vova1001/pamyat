-- name: AddQuestion :one
INSERT INTO questions (source, name, contact_kind, contact, body, tg_chat_id, tg_message_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tg_chat_id, tg_message_id) WHERE tg_chat_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: SetQuestionOwnerMessage :exec
UPDATE questions SET owner_message_id = $2 WHERE id = $1;

-- name: GetQuestion :one
SELECT * FROM questions WHERE id = $1;

-- name: QuestionByOwnerMessage :one
SELECT * FROM questions WHERE owner_message_id = $1 ORDER BY id DESC LIMIT 1;

-- name: ListQuestions :many
SELECT * FROM questions ORDER BY created_at DESC, id DESC LIMIT $1;

-- name: AnswerQuestion :one
UPDATE questions SET answer = $2, answered_via = $3, answered_at = now() WHERE id = $1
RETURNING *;

-- name: LastQuestionFromChat :one
SELECT created_at FROM questions WHERE tg_chat_id = $1 AND id <> $2 ORDER BY id DESC LIMIT 1;
