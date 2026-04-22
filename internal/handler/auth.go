package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/axlsoft/lantern/internal/authz"
	"github.com/axlsoft/lantern/internal/config"
	"github.com/axlsoft/lantern/internal/db/generated"
	"github.com/axlsoft/lantern/internal/httperr"
	"github.com/axlsoft/lantern/internal/mailer"
	"github.com/axlsoft/lantern/internal/pgconv"
	"github.com/axlsoft/lantern/internal/tenancy"
)

// AuthHandler handles all local authentication endpoints.
type AuthHandler struct {
	pool   *pgxpool.Pool
	q      *generated.Queries
	mailer mailer.Sender
	cfg    *config.Config
}

// NewAuthHandler constructs an AuthHandler.
func NewAuthHandler(pool *pgxpool.Pool, m *mailer.Mailer, cfg *config.Config) *AuthHandler {
	return &AuthHandler{pool: pool, q: generated.New(pool), mailer: m, cfg: cfg}
}

// NewAuthHandlerWithMailer constructs an AuthHandler with any mailer.Sender (e.g. a test stub).
func NewAuthHandlerWithMailer(pool *pgxpool.Pool, m mailer.Sender, cfg *config.Config) *AuthHandler {
	return &AuthHandler{pool: pool, q: generated.New(pool), mailer: m, cfg: cfg}
}

// ── Signup ────────────────────────────────────────────────────────────────────

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid JSON body")
		return
	}
	if req.Email == "" || req.Password == "" {
		httperr.BadRequest(w, "email and password are required")
		return
	}
	if len(req.Password) < 8 {
		httperr.BadRequest(w, "password must be at least 8 characters")
		return
	}

	hash, err := authz.HashPassword(req.Password)
	if err != nil {
		httperr.Internal(w, "could not hash password")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	q := generated.New(tx)

	user, err := q.CreateUser(r.Context(), generated.CreateUserParams{
		Email:        req.Email,
		PasswordHash: hash,
	})
	if err != nil {
		if isUniqueViolation(err) {
			httperr.Conflict(w, "email already registered")
			return
		}
		httperr.Internal(w, "could not create user")
		return
	}

	org, err := q.CreateOrganization(r.Context(), req.Email+"'s Organization")
	if err != nil {
		httperr.Internal(w, "could not create organization")
		return
	}

	if err := q.AddOrgMember(r.Context(), generated.AddOrgMemberParams{
		UserID:         user.ID,
		OrganizationID: org.ID,
		Role:           generated.OrgRoleOwner,
	}); err != nil {
		httperr.Internal(w, "could not assign org owner")
		return
	}

	verification, err := q.CreateEmailVerification(r.Context(), generated.CreateEmailVerificationParams{
		UserID:    user.ID,
		ExpiresAt: pgconv.Timestamptz(time.Now().Add(24 * time.Hour)),
	})
	if err != nil {
		httperr.Internal(w, "could not create verification token")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}

	go func() {
		_ = h.mailer.SendVerification(user.Email, pgconv.FromUUID(verification.Token).String())
	}()

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"user_id": pgconv.FromUUID(user.ID),
			"message": "verification email sent",
		},
	})
}

// ── Verify email ──────────────────────────────────────────────────────────────

func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		httperr.BadRequest(w, "token is required")
		return
	}

	tokenUUID, err := parseUUID(token)
	if err != nil {
		httperr.BadRequest(w, "invalid token")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	q := generated.New(tx)

	v, err := q.ConsumeEmailVerification(r.Context(), pgconv.UUID(tokenUUID))
	if err != nil {
		httperr.BadRequest(w, "token not found, expired, or already used")
		return
	}

	if err := q.SetEmailVerified(r.Context(), v.UserID); err != nil {
		httperr.Internal(w, "could not verify email")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"message": "email verified"}})
}

// ── Login ─────────────────────────────────────────────────────────────────────

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid JSON body")
		return
	}

	user, err := h.q.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		_ = authz.CheckPassword(req.Password, "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		httperr.Unauthorized(w, "invalid email or password")
		return
	}

	if err := authz.CheckPassword(req.Password, user.PasswordHash); err != nil {
		httperr.Unauthorized(w, "invalid email or password")
		return
	}

	if !user.EmailVerifiedAt.Valid {
		httperr.Unauthorized(w, "email address not yet verified")
		return
	}

	ua := r.UserAgent()
	session, err := h.q.CreateSession(r.Context(), generated.CreateSessionParams{
		UserID:    user.ID,
		ExpiresAt: pgconv.Timestamptz(time.Now().Add(h.cfg.SessionDuration)),
		UserAgent: &ua,
		IpAddress: pgconv.Addr(extractClientIP(r)),
	})
	if err != nil {
		httperr.Internal(w, "could not create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "lantern_session",
		Value:    pgconv.FromUUID(session.ID).String(),
		Path:     "/",
		Expires:  time.Now().Add(h.cfg.SessionDuration),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.cfg.Env != "development",
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"user_id":    pgconv.FromUUID(user.ID),
			"session_id": pgconv.FromUUID(session.ID),
		},
	})
}

// ── Logout ────────────────────────────────────────────────────────────────────

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("lantern_session")
	if err == nil {
		if sessionID, err := parseUUID(cookie.Value); err == nil {
			_ = h.q.DeleteSession(r.Context(), pgconv.UUID(sessionID))
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "lantern_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"message": "logged out"}})
}

// ── Password reset ────────────────────────────────────────────────────────────

type passwordResetRequestBody struct {
	Email string `json:"email"`
}

func (h *AuthHandler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req passwordResetRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid JSON body")
		return
	}

	user, err := h.q.GetUserByEmail(r.Context(), req.Email)
	if err == nil {
		reset, err := h.q.CreatePasswordReset(r.Context(), generated.CreatePasswordResetParams{
			UserID:    user.ID,
			ExpiresAt: pgconv.Timestamptz(time.Now().Add(time.Hour)),
		})
		if err == nil {
			go func() {
				_ = h.mailer.SendPasswordReset(user.Email, pgconv.FromUUID(reset.Token).String())
			}()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"message": "if the email is registered, a reset link was sent"},
	})
}

type completePasswordResetRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (h *AuthHandler) CompletePasswordReset(w http.ResponseWriter, r *http.Request) {
	var req completePasswordResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid JSON body")
		return
	}
	if len(req.NewPassword) < 8 {
		httperr.BadRequest(w, "password must be at least 8 characters")
		return
	}

	tokenUUID, err := parseUUID(req.Token)
	if err != nil {
		httperr.BadRequest(w, "invalid token")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	q := generated.New(tx)

	reset, err := q.ConsumePasswordReset(r.Context(), pgconv.UUID(tokenUUID))
	if err != nil {
		httperr.BadRequest(w, "token not found, expired, or already used")
		return
	}

	hash, err := authz.HashPassword(req.NewPassword)
	if err != nil {
		httperr.Internal(w, "could not hash password")
		return
	}

	if err := q.UpdatePasswordHash(r.Context(), generated.UpdatePasswordHashParams{
		ID:           reset.UserID,
		PasswordHash: hash,
	}); err != nil {
		httperr.Internal(w, "could not update password")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"message": "password updated"}})
}

// ── /me ───────────────────────────────────────────────────────────────────────

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("lantern_session")
	if err != nil {
		httperr.Unauthorized(w, "not authenticated")
		return
	}

	sessionID, err := parseUUID(cookie.Value)
	if err != nil {
		httperr.Unauthorized(w, "invalid session")
		return
	}

	session, err := h.q.GetSession(r.Context(), pgconv.UUID(sessionID))
	if err != nil {
		httperr.Unauthorized(w, "session not found or expired")
		return
	}

	go func() { _ = h.q.TouchSession(r.Context(), session.ID) }()

	rows, err := h.q.GetUserWithOrgs(r.Context(), session.UserID)
	if err != nil || len(rows) == 0 {
		httperr.Internal(w, "could not load user")
		return
	}

	orgs := make([]map[string]any, 0)
	for _, row := range rows {
		if !row.OrganizationID.Valid {
			continue
		}
		entry := map[string]any{
			"id":   pgconv.FromUUID(row.OrganizationID),
			"name": row.OrgName,
		}
		if row.OrgRole != nil {
			entry["role"] = *row.OrgRole
		}
		orgs = append(orgs, entry)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":                pgconv.FromUUID(rows[0].ID),
			"email":             rows[0].Email,
			"email_verified_at": rows[0].EmailVerifiedAt,
			"created_at":        rows[0].CreatedAt,
			"organizations":     orgs,
		},
	})
}

// ── Session middleware ────────────────────────────────────────────────────────

// SessionMiddleware populates org/user context from the session cookie.
func (h *AuthHandler) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("lantern_session")
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		sessionID, err := parseUUID(cookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		session, err := h.q.GetSession(r.Context(), pgconv.UUID(sessionID))
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		rows, err := h.q.GetUserWithOrgs(r.Context(), session.UserID)
		if err != nil || len(rows) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		ctx := tenancyCtx(r.Context(), pgconv.FromUUID(rows[0].ID), rows)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireSession is a middleware that returns 401 if there is no authenticated user.
func RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := tenancy.UserFromContext(r.Context()); !ok {
			httperr.Unauthorized(w, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
