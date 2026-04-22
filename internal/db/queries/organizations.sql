-- name: CreateOrganization :one
INSERT INTO organizations (name)
VALUES ($1)
RETURNING *;

-- name: GetOrganization :one
SELECT * FROM organizations WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: UpdateOrganization :one
UPDATE organizations SET name = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteOrganization :exec
UPDATE organizations SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: AddOrgMember :exec
INSERT INTO organization_memberships (user_id, organization_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, organization_id) DO UPDATE SET role = EXCLUDED.role;

-- name: GetOrgMembership :one
SELECT * FROM organization_memberships
WHERE user_id = $1 AND organization_id = $2
LIMIT 1;

-- name: ListOrgMembers :many
SELECT om.*, u.email FROM organization_memberships om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = $1;

-- name: CreateOrgInvite :one
INSERT INTO org_invites (organization_id, invited_email, role, invited_by, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetOrgInvite :one
SELECT * FROM org_invites
WHERE token = $1 AND expires_at > now() AND accepted_at IS NULL
LIMIT 1;

-- name: AcceptOrgInvite :exec
UPDATE org_invites SET accepted_at = now() WHERE token = $1;
