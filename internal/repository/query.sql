-- name: FindAllForUser :many
SELECT id, type, title, message, link, created_at, read_at, extra, user_id, event_id, type_label
FROM notification
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: FindLatestForUser :many
SELECT id, type, title, message, link, created_at, read_at, extra, user_id, event_id, type_label
FROM notification
WHERE user_id = $1 AND read_at IS NULL
ORDER BY created_at DESC
LIMIT $2;

-- name: CountAllForUser :one
SELECT COUNT(*) FROM notification WHERE user_id = $1;

-- name: CountUnreadForUser :one
SELECT COUNT(*) FROM notification
WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkAsRead :exec
UPDATE notification
SET read_at = NOW()
WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllAsReadForUser :exec
UPDATE notification
SET read_at = NOW()
WHERE user_id = $1 AND read_at IS NULL;

-- Повтор события (ретрай продюсера, redelivery после nack) не должен давать
-- второе уведомление. Предикат WHERE обязателен: индекс частичный, без него
-- Postgres не выведет его для ON CONFLICT. При конфликте строка не
-- возвращается — репозиторий трактует это как «уже создано», а не как ошибку.
-- name: CreateNotification :one
INSERT INTO notification (event_id, type, type_label, title, message, link, created_at, read_at, extra, user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (event_id, user_id) WHERE event_id IS NOT NULL DO NOTHING
RETURNING id, type, title, message, link, created_at, read_at, extra, user_id, event_id, type_label;
