package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/axlsoft/lantern/internal/db/generated"
	"github.com/axlsoft/lantern/internal/pgconv"
	"github.com/axlsoft/lantern/internal/tenancy"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func extractClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return fwd
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// tenancyCtx sets user/org context from the user's org memberships.
// The first owner-role org wins; otherwise the first membership is used.
func tenancyCtx(ctx context.Context, userID uuid.UUID, rows []generated.GetUserWithOrgsRow) context.Context {
	ctx = tenancy.WithUserID(ctx, userID)

	var orgID uuid.UUID
	for _, row := range rows {
		if !row.OrganizationID.Valid {
			continue
		}
		id := pgconv.FromUUID(row.OrganizationID)
		if orgID == uuid.Nil {
			orgID = id
		}
		if row.OrgRole != nil && *row.OrgRole == generated.OrgRoleOwner {
			orgID = id
			break
		}
	}

	if orgID != uuid.Nil {
		ctx = tenancy.WithOrgID(ctx, orgID)
	}
	return ctx
}

// beginOrgTx opens a transaction and sets the org-scoped RLS parameter.
// The caller is responsible for calling tx.Rollback (typically via defer).
func beginOrgTx(ctx context.Context, pool *pgxpool.Pool, orgID uuid.UUID) (pgx.Tx, *generated.Queries, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL lantern.current_organization_id = '%s'", orgID)); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}
	return tx, generated.New(tx), nil
}

// lookupTeamOrg resolves the organization_id for a team, verifying the user
// is a member of that org. Returns an error if team not found or user not member.
func lookupTeamOrg(ctx context.Context, pool *pgxpool.Pool, teamID, userID uuid.UUID) (uuid.UUID, error) {
	var orgID pgtype.UUID
	err := pool.QueryRow(ctx, "SELECT get_team_org_id($1, $2)", teamID, userID).Scan(&orgID)
	if err != nil || !orgID.Valid {
		return uuid.Nil, errors.New("not found")
	}
	return pgconv.FromUUID(orgID), nil
}

// lookupProjectOrg resolves the organization_id for a project, verifying the
// user is a member of that org. Returns an error if project not found or user not member.
func lookupProjectOrg(ctx context.Context, pool *pgxpool.Pool, projectID, userID uuid.UUID) (uuid.UUID, error) {
	var orgID pgtype.UUID
	err := pool.QueryRow(ctx, "SELECT get_project_org_id($1, $2)", projectID, userID).Scan(&orgID)
	if err != nil || !orgID.Valid {
		return uuid.Nil, errors.New("not found")
	}
	return pgconv.FromUUID(orgID), nil
}
