-- name: CreateProject :one
INSERT INTO projects (organization_id, team_id, name, slug, default_branch, github_repo_full_name)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetProject :one
SELECT * FROM projects WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: GetProjectBySlug :one
SELECT * FROM projects
WHERE organization_id = $1 AND slug = $2 AND deleted_at IS NULL
LIMIT 1;

-- name: ListProjects :many
SELECT * FROM projects
WHERE organization_id = $1 AND deleted_at IS NULL
ORDER BY name;

-- name: UpdateProject :one
UPDATE projects
SET name = $2, github_repo_full_name = $3, default_branch = $4, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteProject :exec
UPDATE projects SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;
