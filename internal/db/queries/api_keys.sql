-- name: CreateAPIKey :one
INSERT INTO api_keys (project_id, organization_id, name, key_hash, key_prefix, created_by_user_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListAPIKeys :many
SELECT * FROM api_keys WHERE project_id = $1 ORDER BY created_at DESC;

-- name: GetAPIKeyByPrefix :one
SELECT * FROM api_keys WHERE key_prefix = $1 AND revoked_at IS NULL LIMIT 1;

-- name: GetAPIKey :one
SELECT * FROM api_keys WHERE id = $1 LIMIT 1;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET revoked_at = now() WHERE id = $1;

-- name: TouchAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = now() WHERE id = $1;
