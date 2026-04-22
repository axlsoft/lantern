-- name: CreateTeam :one
INSERT INTO teams (organization_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetTeam :one
SELECT * FROM teams WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: ListTeams :many
SELECT * FROM teams WHERE organization_id = $1 AND deleted_at IS NULL ORDER BY name;

-- name: UpdateTeam :one
UPDATE teams SET name = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteTeam :exec
UPDATE teams SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: AddTeamMember :exec
INSERT INTO team_memberships (user_id, team_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, team_id) DO UPDATE SET role = EXCLUDED.role;

-- name: GetTeamMembership :one
SELECT * FROM team_memberships WHERE user_id = $1 AND team_id = $2 LIMIT 1;
