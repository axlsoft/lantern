-- name: CreateUser :one
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 LIMIT 1;

-- name: SetEmailVerified :exec
UPDATE users SET email_verified_at = now(), updated_at = now() WHERE id = $1;

-- name: UpdatePasswordHash :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: GetUserWithOrgs :many
SELECT
    u.id,
    u.email,
    u.email_verified_at,
    u.created_at,
    om.organization_id,
    om.role AS org_role,
    o.name  AS org_name
FROM users u
LEFT JOIN organization_memberships om ON om.user_id = u.id
LEFT JOIN organizations o             ON o.id = om.organization_id AND o.deleted_at IS NULL
WHERE u.id = $1;
