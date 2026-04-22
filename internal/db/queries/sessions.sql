-- name: CreateSession :one
INSERT INTO sessions (user_id, expires_at, user_agent, ip_address)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetSession :one
SELECT s.*, u.email, u.email_verified_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = $1 AND s.expires_at > now()
LIMIT 1;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE id = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < now();
