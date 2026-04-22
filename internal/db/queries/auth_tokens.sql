-- name: CreateEmailVerification :one
INSERT INTO email_verifications (user_id, expires_at)
VALUES ($1, $2)
RETURNING *;

-- name: ConsumeEmailVerification :one
UPDATE email_verifications
SET consumed_at = now()
WHERE token = $1
  AND expires_at > now()
  AND consumed_at IS NULL
RETURNING *;

-- name: CreatePasswordReset :one
INSERT INTO password_resets (user_id, expires_at)
VALUES ($1, $2)
RETURNING *;

-- name: ConsumePasswordReset :one
UPDATE password_resets
SET consumed_at = now()
WHERE token = $1
  AND expires_at > now()
  AND consumed_at IS NULL
RETURNING *;
