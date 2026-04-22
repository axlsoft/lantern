package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/axlsoft/lantern/internal/audit"
	"github.com/axlsoft/lantern/internal/authz"
	"github.com/axlsoft/lantern/internal/config"
	"github.com/axlsoft/lantern/internal/db/generated"
	"github.com/axlsoft/lantern/internal/httperr"
	"github.com/axlsoft/lantern/internal/pgconv"
	"github.com/axlsoft/lantern/internal/tenancy"
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// APIKeyHandler manages API key lifecycle endpoints.
type APIKeyHandler struct {
	pool *pgxpool.Pool
	q    *generated.Queries
	perm *authz.Checker
	cfg  *config.Config
}

// NewAPIKeyHandler constructs an APIKeyHandler.
func NewAPIKeyHandler(pool *pgxpool.Pool, cfg *config.Config) *APIKeyHandler {
	return &APIKeyHandler{pool: pool, q: generated.New(pool), perm: authz.NewChecker(pool), cfg: cfg}
}

// generateKey returns the full key and its short prefix for display.
// Format: lntn_<40 base62 chars>
func generateKey() (full, prefix string, err error) {
	secret, err := randBase62(40)
	if err != nil {
		return
	}
	full = "lntn_" + secret
	prefix = full[:12] // "lntn_" + 7 chars
	return
}

func randBase62(n int) (string, error) {
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(base62Chars))))
		if err != nil {
			return "", err
		}
		b[i] = base62Chars[idx.Int64()]
	}
	return string(b), nil
}

func hashKey(full, pepper string) string {
	h := sha256.Sum256([]byte(full + pepper))
	return fmt.Sprintf("%x", h)
}

func (h *APIKeyHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
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

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		httperr.BadRequest(w, "name is required")
		return
	}

	full, prefix, err := generateKey()
	if err != nil {
		httperr.Internal(w, "could not generate key")
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	key, err := q.CreateAPIKey(r.Context(), generated.CreateAPIKeyParams{
		ProjectID:       pgconv.UUID(projectID),
		OrganizationID:  pgconv.UUID(orgID),
		Name:            body.Name,
		KeyHash:         hashKey(full, h.cfg.APIKeyPepper),
		KeyPrefix:       prefix,
		CreatedByUserID: pgconv.UUID(userID),
	})
	if err != nil {
		httperr.Internal(w, "could not create API key")
		return
	}

	audit.Log(tenancy.WithOrgID(r.Context(), orgID), q, r, "apikey.create", "api_key", pgconv.FromUUID(key.ID).String(), map[string]any{"prefix": prefix})
	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"id":         pgconv.FromUUID(key.ID),
			"name":       key.Name,
			"key":        full, // full key returned only at creation
			"prefix":     key.KeyPrefix,
			"created_at": key.CreatedAt,
		},
	})
}

func (h *APIKeyHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
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

	keys, err := q.ListAPIKeys(r.Context(), pgconv.UUID(projectID))
	if err != nil {
		httperr.Internal(w, "could not list API keys")
		return
	}

	out := make([]map[string]any, len(keys))
	for i, k := range keys {
		out[i] = map[string]any{
			"id":           pgconv.FromUUID(k.ID),
			"name":         k.Name,
			"prefix":       k.KeyPrefix,
			"created_at":   k.CreatedAt,
			"last_used_at": k.LastUsedAt,
			"revoked_at":   k.RevokedAt,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *APIKeyHandler) RotateAPIKey(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}
	keyID, err := parseUUID(chi.URLParam(r, "key_id"))
	if err != nil {
		httperr.NotFound(w, "key not found")
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

	full, prefix, err := generateKey()
	if err != nil {
		httperr.Internal(w, "could not generate key")
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	oldKey, err := q.GetAPIKey(r.Context(), pgconv.UUID(keyID))
	if err != nil {
		httperr.NotFound(w, "key not found")
		return
	}

	if err := q.RevokeAPIKey(r.Context(), pgconv.UUID(keyID)); err != nil {
		httperr.Internal(w, "could not revoke key")
		return
	}

	newKey, err := q.CreateAPIKey(r.Context(), generated.CreateAPIKeyParams{
		ProjectID:       pgconv.UUID(projectID),
		OrganizationID:  pgconv.UUID(orgID),
		Name:            oldKey.Name + " (rotated)",
		KeyHash:         hashKey(full, h.cfg.APIKeyPepper),
		KeyPrefix:       prefix,
		CreatedByUserID: pgconv.UUID(userID),
	})
	if err != nil {
		httperr.Internal(w, "could not create replacement key")
		return
	}

	audit.Log(tenancy.WithOrgID(r.Context(), orgID), q, r, "apikey.rotate", "api_key", keyID.String(), map[string]any{"new_prefix": prefix})
	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"id":     pgconv.FromUUID(newKey.ID),
			"name":   newKey.Name,
			"key":    full,
			"prefix": newKey.KeyPrefix,
		},
	})
}

func (h *APIKeyHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}
	keyID, err := parseUUID(chi.URLParam(r, "key_id"))
	if err != nil {
		httperr.NotFound(w, "key not found")
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

	if err := q.RevokeAPIKey(r.Context(), pgconv.UUID(keyID)); err != nil {
		httperr.Internal(w, "could not revoke key")
		return
	}

	audit.Log(tenancy.WithOrgID(r.Context(), orgID), q, r, "apikey.revoke", "api_key", keyID.String(), nil)
	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"message": "key revoked"}})
}

// ── API key auth middleware ────────────────────────────────────────────────────

// APIKeyMiddleware authenticates requests using a Bearer token API key.
// Uses get_api_key_by_prefix and touch_api_key_last_used SECURITY DEFINER
// functions to bypass RLS (the org is unknown at authentication time).
func APIKeyMiddleware(pool *pgxpool.Pool, cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
				httperr.Unauthorized(w, "Bearer token required")
				return
			}
			full := authHeader[7:]
			if len(full) < 12 {
				httperr.Unauthorized(w, "invalid API key")
				return
			}
			prefix := full[:12]

			var keyID, orgID pgtype.UUID
			var keyHash string
			err := pool.QueryRow(r.Context(),
				`SELECT id, organization_id, key_hash FROM get_api_key_by_prefix($1)`,
				prefix,
			).Scan(&keyID, &orgID, &keyHash)
			if err != nil {
				httperr.Unauthorized(w, "invalid or revoked API key")
				return
			}

			if hashKey(full, cfg.APIKeyPepper) != keyHash {
				httperr.Unauthorized(w, "invalid or revoked API key")
				return
			}

			go func() {
				_, _ = pool.Exec(context.Background(),
					`SELECT touch_api_key_last_used($1)`, keyID)
			}()

			ctx := tenancy.WithOrgID(r.Context(), pgconv.FromUUID(orgID))
			ctx = tenancy.WithAPIKeyID(ctx, pgconv.FromUUID(keyID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
