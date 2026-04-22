package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/axlsoft/lantern/internal/audit"
	"github.com/axlsoft/lantern/internal/authz"
	"github.com/axlsoft/lantern/internal/config"
	"github.com/axlsoft/lantern/internal/db/generated"
	"github.com/axlsoft/lantern/internal/httperr"
	"github.com/axlsoft/lantern/internal/mailer"
	"github.com/axlsoft/lantern/internal/pgconv"
	"github.com/axlsoft/lantern/internal/tenancy"
)

// OrgHandler handles org/team/project management endpoints.
type OrgHandler struct {
	pool   *pgxpool.Pool
	q      *generated.Queries
	perm   *authz.Checker
	mailer mailer.Sender
	cfg    *config.Config
}

// NewOrgHandler constructs an OrgHandler.
func NewOrgHandler(pool *pgxpool.Pool, m mailer.Sender, cfg *config.Config) *OrgHandler {
	return &OrgHandler{
		pool:   pool,
		q:      generated.New(pool),
		perm:   authz.NewChecker(pool),
		mailer: m,
		cfg:    cfg,
	}
}

// ── Organizations ─────────────────────────────────────────────────────────────
// organizations table has no RLS; pool-level queries are fine here.

func (h *OrgHandler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		httperr.BadRequest(w, "name is required")
		return
	}

	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "authentication required")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	q := generated.New(tx)

	org, err := q.CreateOrganization(r.Context(), body.Name)
	if err != nil {
		httperr.Internal(w, "could not create organization")
		return
	}

	if err := q.AddOrgMember(r.Context(), generated.AddOrgMemberParams{
		UserID:         pgconv.UUID(userID),
		OrganizationID: org.ID,
		Role:           generated.OrgRoleOwner,
	}); err != nil {
		httperr.Internal(w, "could not assign owner")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": org})
}

func (h *OrgHandler) GetOrg(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseUUID(chi.URLParam(r, "org_id"))
	if err != nil {
		httperr.NotFound(w, "organization not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermOrgRead); err != nil {
		writePermError(w, err)
		return
	}
	org, err := h.q.GetOrganization(r.Context(), pgconv.UUID(orgID))
	if err != nil {
		httperr.NotFound(w, "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": org})
}

func (h *OrgHandler) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseUUID(chi.URLParam(r, "org_id"))
	if err != nil {
		httperr.NotFound(w, "organization not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermOrgWrite); err != nil {
		writePermError(w, err)
		return
	}

	var body struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		httperr.BadRequest(w, "name is required")
		return
	}

	org, err := h.q.UpdateOrganization(r.Context(), generated.UpdateOrganizationParams{
		ID:   pgconv.UUID(orgID),
		Name: body.Name,
	})
	if err != nil {
		httperr.NotFound(w, "organization not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": org})
}

func (h *OrgHandler) DeleteOrg(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseUUID(chi.URLParam(r, "org_id"))
	if err != nil {
		httperr.NotFound(w, "organization not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermOrgDelete); err != nil {
		writePermError(w, err)
		return
	}
	if err := h.q.SoftDeleteOrganization(r.Context(), pgconv.UUID(orgID)); err != nil {
		httperr.Internal(w, "could not delete organization")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"message": "organization deleted"}})
}

func (h *OrgHandler) InviteToOrg(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseUUID(chi.URLParam(r, "org_id"))
	if err != nil {
		httperr.NotFound(w, "organization not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermOrgInvite); err != nil {
		writePermError(w, err)
		return
	}

	var body struct {
		Email string            `json:"email"`
		Role  generated.OrgRole `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		httperr.BadRequest(w, "email is required")
		return
	}
	if body.Role == "" {
		body.Role = generated.OrgRoleMember
	}

	userID, _ := tenancy.UserFromContext(r.Context())

	org, err := h.q.GetOrganization(r.Context(), pgconv.UUID(orgID))
	if err != nil {
		httperr.NotFound(w, "organization not found")
		return
	}

	invite, err := h.q.CreateOrgInvite(r.Context(), generated.CreateOrgInviteParams{
		OrganizationID: pgconv.UUID(orgID),
		InvitedEmail:   body.Email,
		Role:           body.Role,
		InvitedBy:      pgconv.UUID(userID),
		ExpiresAt:      pgconv.Timestamptz(time.Now().Add(7 * 24 * time.Hour)),
	})
	if err != nil {
		httperr.Internal(w, "could not create invite")
		return
	}

	go func() {
		_ = h.mailer.SendOrgInvite(body.Email, org.Name, pgconv.FromUUID(invite.Token).String())
	}()

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"token": invite.Token, "expires_at": invite.ExpiresAt}})
}

func (h *OrgHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	token, err := parseUUID(chi.URLParam(r, "token"))
	if err != nil {
		httperr.NotFound(w, "invite not found")
		return
	}

	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "authentication required")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	q := generated.New(tx)

	invite, err := q.GetOrgInvite(r.Context(), pgconv.UUID(token))
	if err != nil {
		httperr.NotFound(w, "invite not found or expired")
		return
	}

	if err := q.AddOrgMember(r.Context(), generated.AddOrgMemberParams{
		UserID:         pgconv.UUID(userID),
		OrganizationID: invite.OrganizationID,
		Role:           invite.Role,
	}); err != nil {
		httperr.Internal(w, "could not add member")
		return
	}

	if err := q.AcceptOrgInvite(r.Context(), pgconv.UUID(token)); err != nil {
		httperr.Internal(w, "could not mark invite accepted")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"message": "invite accepted"}})
}

// ── Teams ─────────────────────────────────────────────────────────────────────

func (h *OrgHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseUUID(chi.URLParam(r, "org_id"))
	if err != nil {
		httperr.NotFound(w, "organization not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermTeamWrite); err != nil {
		writePermError(w, err)
		return
	}

	var body struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		httperr.BadRequest(w, "name is required")
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	team, err := q.CreateTeam(r.Context(), generated.CreateTeamParams{
		OrganizationID: pgconv.UUID(orgID),
		Name:           body.Name,
	})
	if err != nil {
		httperr.Internal(w, "could not create team")
		return
	}

	auditCtx := tenancy.WithOrgID(r.Context(), orgID)
	audit.Log(auditCtx, q, r, "team.create", "team", pgconv.FromUUID(team.ID).String(), nil)
	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": team})
}

func (h *OrgHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseUUID(chi.URLParam(r, "org_id"))
	if err != nil {
		httperr.NotFound(w, "organization not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermTeamRead); err != nil {
		writePermError(w, err)
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	teams, err := q.ListTeams(r.Context(), pgconv.UUID(orgID))
	if err != nil {
		httperr.Internal(w, "could not list teams")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": teams})
}

func (h *OrgHandler) GetTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseUUID(chi.URLParam(r, "team_id"))
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "authentication required")
		return
	}
	orgID, err := lookupTeamOrg(r.Context(), h.pool, teamID, userID)
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermTeamRead); err != nil {
		writePermError(w, err)
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	team, err := q.GetTeam(r.Context(), pgconv.UUID(teamID))
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": team})
}

func (h *OrgHandler) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseUUID(chi.URLParam(r, "team_id"))
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "authentication required")
		return
	}
	orgID, err := lookupTeamOrg(r.Context(), h.pool, teamID, userID)
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermTeamWrite); err != nil {
		writePermError(w, err)
		return
	}

	var body struct{ Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		httperr.BadRequest(w, "name is required")
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	updated, err := q.UpdateTeam(r.Context(), generated.UpdateTeamParams{
		ID:   pgconv.UUID(teamID),
		Name: body.Name,
	})
	if err != nil {
		httperr.Internal(w, "could not update team")
		return
	}

	audit.Log(tenancy.WithOrgID(r.Context(), orgID), q, r, "team.update", "team", teamID.String(), map[string]any{"name": body.Name})
	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (h *OrgHandler) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseUUID(chi.URLParam(r, "team_id"))
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "authentication required")
		return
	}
	orgID, err := lookupTeamOrg(r.Context(), h.pool, teamID, userID)
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermTeamDelete); err != nil {
		writePermError(w, err)
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	if err := q.SoftDeleteTeam(r.Context(), pgconv.UUID(teamID)); err != nil {
		httperr.Internal(w, "could not delete team")
		return
	}

	audit.Log(tenancy.WithOrgID(r.Context(), orgID), q, r, "team.delete", "team", teamID.String(), nil)
	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"message": "team deleted"}})
}

func (h *OrgHandler) AddTeamMember(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseUUID(chi.URLParam(r, "team_id"))
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "authentication required")
		return
	}
	orgID, err := lookupTeamOrg(r.Context(), h.pool, teamID, userID)
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermTeamMemberWrite); err != nil {
		writePermError(w, err)
		return
	}

	var body struct {
		UserID string             `json:"user_id"`
		Role   generated.TeamRole `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID == "" {
		httperr.BadRequest(w, "user_id is required")
		return
	}
	uid, err := parseUUID(body.UserID)
	if err != nil {
		httperr.BadRequest(w, "invalid user_id")
		return
	}
	if body.Role == "" {
		body.Role = generated.TeamRoleMember
	}

	if err := h.q.AddTeamMember(r.Context(), generated.AddTeamMemberParams{
		UserID: pgconv.UUID(uid),
		TeamID: pgconv.UUID(teamID),
		Role:   body.Role,
	}); err != nil {
		httperr.Internal(w, "could not add team member")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"message": "member added"}})
}

// ── Projects ──────────────────────────────────────────────────────────────────

func (h *OrgHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	teamID, err := parseUUID(chi.URLParam(r, "team_id"))
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "authentication required")
		return
	}
	orgID, err := lookupTeamOrg(r.Context(), h.pool, teamID, userID)
	if err != nil {
		httperr.NotFound(w, "team not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermProjectWrite); err != nil {
		writePermError(w, err)
		return
	}

	var body struct {
		Name               string `json:"name"`
		Slug               string `json:"slug"`
		DefaultBranch      string `json:"default_branch"`
		GithubRepoFullName string `json:"github_repo_full_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.Slug == "" {
		httperr.BadRequest(w, "name and slug are required")
		return
	}
	if body.DefaultBranch == "" {
		body.DefaultBranch = "main"
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	project, err := q.CreateProject(r.Context(), generated.CreateProjectParams{
		OrganizationID:     pgconv.UUID(orgID),
		TeamID:             pgconv.UUID(teamID),
		Name:               body.Name,
		Slug:               body.Slug,
		DefaultBranch:      body.DefaultBranch,
		GithubRepoFullName: &body.GithubRepoFullName,
	})
	if err != nil {
		if isUniqueViolation(err) {
			httperr.Conflict(w, "a project with that slug already exists in this organization")
			return
		}
		httperr.Internal(w, "could not create project")
		return
	}

	audit.Log(tenancy.WithOrgID(r.Context(), orgID), q, r, "project.create", "project", pgconv.FromUUID(project.ID).String(), nil)
	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": project})
}

func (h *OrgHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "authentication required")
		return
	}
	orgID, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID)
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermProjectRead); err != nil {
		writePermError(w, err)
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	project, err := q.GetProject(r.Context(), pgconv.UUID(projectID))
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": project})
}

func (h *OrgHandler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "authentication required")
		return
	}
	orgID, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID)
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermProjectWrite); err != nil {
		writePermError(w, err)
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	project, err := q.GetProject(r.Context(), pgconv.UUID(projectID))
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}

	var body struct {
		Name               string `json:"name"`
		GithubRepoFullName string `json:"github_repo_full_name"`
		DefaultBranch      string `json:"default_branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperr.BadRequest(w, "invalid JSON body")
		return
	}
	if body.Name == "" {
		body.Name = project.Name
	}
	if body.DefaultBranch == "" {
		body.DefaultBranch = project.DefaultBranch
	}

	updated, err := q.UpdateProject(r.Context(), generated.UpdateProjectParams{
		ID:                 pgconv.UUID(projectID),
		Name:               body.Name,
		GithubRepoFullName: &body.GithubRepoFullName,
		DefaultBranch:      body.DefaultBranch,
	})
	if err != nil {
		httperr.Internal(w, "could not update project")
		return
	}

	audit.Log(tenancy.WithOrgID(r.Context(), orgID), q, r, "project.update", "project", projectID.String(), nil)
	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

func (h *OrgHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "authentication required")
		return
	}
	orgID, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID)
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}
	if err := h.perm.RequireOrgID(r.Context(), orgID, authz.PermProjectDelete); err != nil {
		writePermError(w, err)
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	if err := q.SoftDeleteProject(r.Context(), pgconv.UUID(projectID)); err != nil {
		httperr.Internal(w, "could not delete project")
		return
	}

	audit.Log(tenancy.WithOrgID(r.Context(), orgID), q, r, "project.delete", "project", projectID.String(), nil)
	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"message": "project deleted"}})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writePermError(w http.ResponseWriter, err error) {
	if authz.HTTPStatus(err) == 404 {
		httperr.NotFound(w, "not found")
	} else {
		httperr.Forbidden(w, "insufficient permissions")
	}
}
