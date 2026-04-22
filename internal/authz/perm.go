package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/axlsoft/lantern/internal/db/generated"
	"github.com/axlsoft/lantern/internal/pgconv"
	"github.com/axlsoft/lantern/internal/tenancy"
)

// Permission represents a named capability.
type Permission int

const (
	PermOrgRead Permission = iota
	PermOrgWrite
	PermOrgDelete
	PermOrgInvite
	PermTeamRead
	PermTeamWrite
	PermTeamDelete
	PermTeamMemberWrite
	PermProjectRead
	PermProjectWrite
	PermProjectDelete
)

var orgRolePermissions = map[generated.OrgRole][]Permission{
	generated.OrgRoleOwner:  {PermOrgRead, PermOrgWrite, PermOrgDelete, PermOrgInvite, PermTeamRead, PermTeamWrite, PermTeamDelete, PermTeamMemberWrite, PermProjectRead, PermProjectWrite, PermProjectDelete},
	generated.OrgRoleAdmin:  {PermOrgRead, PermOrgWrite, PermOrgInvite, PermTeamRead, PermTeamWrite, PermTeamDelete, PermTeamMemberWrite, PermProjectRead, PermProjectWrite, PermProjectDelete},
	generated.OrgRoleMember: {PermOrgRead, PermTeamRead, PermProjectRead, PermProjectWrite},
	generated.OrgRoleViewer: {PermOrgRead, PermTeamRead, PermProjectRead},
}

// ErrForbidden is returned when the caller lacks the required permission.
var ErrForbidden = errors.New("forbidden")

// ErrNotFound is returned to avoid tenant enumeration (caller not in org).
var ErrNotFound = errors.New("not found")

// Checker performs permission checks against the database.
type Checker struct {
	pool *pgxpool.Pool
	q    *generated.Queries
}

// NewChecker creates a permission Checker.
func NewChecker(pool *pgxpool.Pool) *Checker {
	return &Checker{pool: pool, q: generated.New(pool)}
}

// RequireOrg checks that the calling user has the given permission in the org
// resolved from the request context. Returns ErrNotFound if the user has no membership
// (to prevent enumeration), ErrForbidden if they lack the specific permission.
func (c *Checker) RequireOrg(ctx context.Context, perm Permission) error {
	userID, ok := tenancy.UserFromContext(ctx)
	if !ok {
		return ErrForbidden
	}
	orgID, err := tenancy.OrgFromContext(ctx)
	if err != nil {
		return ErrNotFound
	}
	return c.checkOrgPerm(ctx, userID, orgID, perm)
}

// RequireOrgID checks permissions for a specific org ID, returning ErrNotFound
// if the caller has no membership (prevents enumeration).
func (c *Checker) RequireOrgID(ctx context.Context, orgID uuid.UUID, perm Permission) error {
	userID, ok := tenancy.UserFromContext(ctx)
	if !ok {
		return ErrForbidden
	}
	return c.checkOrgPerm(ctx, userID, orgID, perm)
}

func (c *Checker) checkOrgPerm(ctx context.Context, userID, orgID uuid.UUID, perm Permission) error {
	membership, err := c.q.GetOrgMembership(ctx, generated.GetOrgMembershipParams{
		UserID:         pgconv.UUID(userID),
		OrganizationID: pgconv.UUID(orgID),
	})
	if err != nil {
		return ErrNotFound
	}
	for _, p := range orgRolePermissions[membership.Role] {
		if p == perm {
			return nil
		}
	}
	return ErrForbidden
}

// HTTPStatus returns the appropriate HTTP status code for a permission error.
// ErrNotFound → 404, ErrForbidden → 403, anything else → 403.
func HTTPStatus(err error) int {
	if errors.Is(err, ErrNotFound) {
		return 404
	}
	return 403
}
