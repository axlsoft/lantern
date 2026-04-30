package handler

import (
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/axlsoft/lantern/internal/config"
	"github.com/axlsoft/lantern/internal/httperr"
	"github.com/axlsoft/lantern/internal/pgconv"
	"github.com/axlsoft/lantern/internal/tenancy"
)

// GitHubHandler proxies source-file fetches from GitHub to the browser.
// The proxy enforces tenant scope: the requested repo must match the project's
// github_repo_full_name, preventing cross-tenant source leaks.
//
// Route: GET /api/v1/github/repos/:owner/:repo/contents/*path
type GitHubHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewGitHubHandler(pool *pgxpool.Pool, cfg *config.Config) *GitHubHandler {
	return &GitHubHandler{pool: pool, cfg: cfg}
}

func (h *GitHubHandler) ProxyContents(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	filePath := chi.URLParam(r, "*")
	ref := r.URL.Query().Get("ref")

	if owner == "" || repo == "" || filePath == "" {
		httperr.BadRequest(w, "owner, repo, and path are required")
		return
	}
	if ref == "" {
		httperr.BadRequest(w, "ref query param required")
		return
	}
	if err := validateFilePath(filePath); err != nil {
		httperr.BadRequest(w, err.Error())
		return
	}

	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "unauthorized")
		return
	}

	repoFullName := fmt.Sprintf("%s/%s", owner, repo)

	// Verify the requesting user has a project that references this repo.
	// This prevents using the proxy to read arbitrary GitHub repos.
	var exists bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
		   SELECT 1 FROM projects p
		   JOIN organization_memberships om ON om.organization_id = p.organization_id
		   WHERE om.user_id = $1
		     AND p.github_repo_full_name = $2
		     AND p.deleted_at IS NULL
		)`,
		pgconv.UUID(userID), repoFullName,
	).Scan(&exists)
	if !exists {
		httperr.Forbidden(w, "repo not associated with any of your projects")
		return
	}

	// In MVP there is no GitHub App — use unauthenticated requests (public repos only).
	// When the GitHub App is installed (Phase 1.6), swap in the installation token here.
	apiURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		owner, repo, filePath, ref,
	)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, apiURL, nil)
	if err != nil {
		httperr.Internal(w, "failed to build GitHub request")
		return
	}
	req.Header.Set("Accept", "application/vnd.github.raw")
	req.Header.Set("User-Agent", "lantern-collector/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		httperr.Internal(w, "GitHub request failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		httperr.NotFound(w, "file not found in repository")
		return
	}
	if resp.StatusCode != http.StatusOK {
		httperr.Internal(w, fmt.Sprintf("GitHub returned %d", resp.StatusCode))
		return
	}

	// Stream the raw file content back to the client.
	// Do not cache — source must be fetched fresh each time.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, resp.Body)
}
