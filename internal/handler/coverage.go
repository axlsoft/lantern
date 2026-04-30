package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/axlsoft/lantern/internal/config"
	"github.com/axlsoft/lantern/internal/httperr"
	"github.com/axlsoft/lantern/internal/pgconv"
	"github.com/axlsoft/lantern/internal/tenancy"
)

// CoverageHandler serves the UI-facing coverage endpoints:
//
//	GET /api/v1/projects/:project_id/dashboard
//	GET /api/v1/projects/:project_id/runs
//	GET /api/v1/projects/:project_id/runs/:run_id
//	GET /api/v1/projects/:project_id/runs/:run_id/tests
//	GET /api/v1/projects/:project_id/gaps
//	GET /api/v1/projects/:project_id/files/*path
//	GET /api/v1/projects/:project_id/files/*path/lines/:line/tests
//	GET /api/v1/projects/:project_id/prs/:pr_number
type CoverageHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewCoverageHandler(pool *pgxpool.Pool, cfg *config.Config) *CoverageHandler {
	return &CoverageHandler{pool: pool, cfg: cfg}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func userFromReq(r *http.Request) (string, bool) {
	id, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		return "", false
	}
	return id.String(), true
}

func nullStr(t *string) any {
	if t == nil {
		return nil
	}
	return *t
}

func nullInt32(v *int32) any {
	if v == nil {
		return nil
	}
	return *v
}

func fmtTime(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func fmtTimePtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.UTC().Format(time.RFC3339)
	return &s
}

func validateFilePath(p string) error {
	if strings.Contains(p, "..") {
		return fmt.Errorf("invalid file path: contains ..")
	}
	if len(p) == 0 || len(p) > 1024 {
		return fmt.Errorf("invalid file path length")
	}
	return nil
}

// ── Dashboard ─────────────────────────────────────────────────────────────────

func (h *CoverageHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.BadRequest(w, "invalid project_id")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "unauthorized")
		return
	}
	orgID, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID)
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}
	_ = orgID

	ctx := r.Context()

	// ── project ───────────────────────────────────────────────────────────
	var projID, projOrgID, projTeamID pgtype.UUID
	var projName, projSlug, projDefaultBranch string
	var projGithubRepo pgtype.Text
	var projCreatedAt, projUpdatedAt pgtype.Timestamptz
	err = h.pool.QueryRow(ctx,
		`SELECT id, organization_id, team_id, name, slug, default_branch,
		        github_repo_full_name, created_at, updated_at
		   FROM projects WHERE id=$1 AND deleted_at IS NULL`,
		pgconv.UUID(projectID),
	).Scan(&projID, &projOrgID, &projTeamID, &projName, &projSlug,
		&projDefaultBranch, &projGithubRepo, &projCreatedAt, &projUpdatedAt)
	if err != nil {
		httperr.NotFound(w, "project not found")
		return
	}

	projMap := map[string]any{
		"id":                    pgconv.FromUUID(projID).String(),
		"organization_id":       pgconv.FromUUID(projOrgID).String(),
		"team_id":               nilIfZero(projTeamID),
		"name":                  projName,
		"slug":                  projSlug,
		"default_branch":        projDefaultBranch,
		"github_repo_full_name": nilIfTextEmpty(projGithubRepo),
		"created_at":            fmtTime(projCreatedAt),
		"updated_at":            fmtTime(projUpdatedAt),
	}

	// ── latest completed run ──────────────────────────────────────────────
	var latestRunAt *string
	var latestRunCommit string
	var latestRunID pgtype.UUID
	row := h.pool.QueryRow(ctx,
		`SELECT id, commit_sha, started_at FROM runs
		  WHERE project_id=$1 AND status='completed'
		  ORDER BY started_at DESC LIMIT 1`,
		pgconv.UUID(projectID),
	)
	var lsAt pgtype.Timestamptz
	_ = row.Scan(&latestRunID, &latestRunCommit, &lsAt)
	if latestRunID.Valid {
		latestRunAt = fmtTimePtr(lsAt)
	}

	// ── coverage summary ──────────────────────────────────────────────────
	var coveredLines, uncoveredLines int64
	if latestRunID.Valid {
		_ = h.pool.QueryRow(ctx,
			`SELECT
			   COUNT(DISTINCT line_start) FILTER (WHERE hit_count > 0),
			   COUNT(DISTINCT line_start) FILTER (WHERE hit_count = 0)
			 FROM coverage_events WHERE run_id=$1`,
			latestRunID,
		).Scan(&coveredLines, &uncoveredLines)
	}

	// ── recent runs (last 10) ──────────────────────────────────────────────
	recentRows, err := h.pool.Query(ctx,
		`SELECT id, commit_sha, branch, ci_run_id, github_pr_number,
		        status, started_at, completed_at,
		        total_tests, passed_tests, failed_tests, skipped_tests
		   FROM runs WHERE project_id=$1
		   ORDER BY started_at DESC LIMIT 10`,
		pgconv.UUID(projectID),
	)
	if err != nil {
		httperr.Internal(w, err.Error())
		return
	}
	recentRuns, err := collectRuns(recentRows)
	if err != nil {
		httperr.Internal(w, err.Error())
		return
	}

	// ── top gaps (top 10 files by uncovered lines) ─────────────────────────
	gapRows, _ := h.pool.Query(ctx,
		`SELECT file_path,
		        COUNT(DISTINCT line_start) AS uncovered_lines,
		        ROUND(
		            (COUNT(DISTINCT line_start)::NUMERIC /
		             NULLIF(MAX(line_end) - MIN(line_start) + 1, 0)) * 0.5
		            + 0.15
		            + LEAST(COUNT(DISTINCT line_start)::NUMERIC / 200, 0.2)
		        , 2) AS risk_score
		   FROM coverage_events
		  WHERE project_id=$1 AND hit_count=0
		  GROUP BY file_path
		  ORDER BY risk_score DESC, uncovered_lines DESC
		  LIMIT 10`,
		pgconv.UUID(projectID),
	)
	topGaps := collectGaps(gapRows)

	// ── open PRs with coverage ─────────────────────────────────────────────
	prRows, _ := h.pool.Query(ctx,
		`SELECT DISTINCT ON (github_pr_number)
		        id, github_pr_number, commit_sha, status
		   FROM runs
		  WHERE project_id=$1 AND github_pr_number IS NOT NULL
		  ORDER BY github_pr_number, started_at DESC`,
		pgconv.UUID(projectID),
	)
	openPRs := collectPRSummaries(prRows)

	writeJSON(w, http.StatusOK, map[string]any{
		"project": projMap,
		"coverage_summary": map[string]any{
			"total_lines":    coveredLines + uncoveredLines,
			"covered_lines":  coveredLines,
			"uncovered_lines": uncoveredLines,
			"gap_count":      uncoveredLines,
			"last_run_at":    latestRunAt,
		},
		"recent_runs": recentRuns,
		"top_gaps":    topGaps,
		"open_prs":    openPRs,
	})
}

// ── Runs ──────────────────────────────────────────────────────────────────────

func (h *CoverageHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.BadRequest(w, "invalid project_id")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "unauthorized")
		return
	}
	if _, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID); err != nil {
		httperr.NotFound(w, "project not found")
		return
	}

	var offset int64
	if c := r.URL.Query().Get("cursor"); c != "" {
		offset, _ = strconv.ParseInt(c, 10, 64)
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, commit_sha, branch, ci_run_id, github_pr_number,
		        status, started_at, completed_at,
		        total_tests, passed_tests, failed_tests, skipped_tests
		   FROM runs WHERE project_id=$1
		   ORDER BY started_at DESC LIMIT 21 OFFSET $2`,
		pgconv.UUID(projectID), offset,
	)
	if err != nil {
		httperr.Internal(w, err.Error())
		return
	}
	all, err := collectRuns(rows)
	if err != nil {
		httperr.Internal(w, err.Error())
		return
	}

	var nextCursor *string
	if len(all) == 21 {
		next := strconv.FormatInt(offset+20, 10)
		nextCursor = &next
		all = all[:20]
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": all, "next_cursor": nextCursor})
}

func (h *CoverageHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.BadRequest(w, "invalid project_id")
		return
	}
	runID, err := parseUUID(chi.URLParam(r, "run_id"))
	if err != nil {
		httperr.BadRequest(w, "invalid run_id")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "unauthorized")
		return
	}
	if _, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID); err != nil {
		httperr.NotFound(w, "project not found")
		return
	}

	var runProjectID pgtype.UUID
	var commitSha, status string
	var branch, ciRunID *string
	var prNum *int32
	var startedAt, completedAt pgtype.Timestamptz
	var total, passed, failed, skipped int32

	err = h.pool.QueryRow(r.Context(),
		`SELECT project_id, commit_sha, branch, ci_run_id, github_pr_number,
		        status, started_at, completed_at,
		        total_tests, passed_tests, failed_tests, skipped_tests
		   FROM runs WHERE id=$1`,
		pgconv.UUID(runID),
	).Scan(&runProjectID, &commitSha, &branch, &ciRunID, &prNum,
		&status, &startedAt, &completedAt, &total, &passed, &failed, &skipped)
	if err != nil || pgconv.FromUUID(runProjectID) != projectID {
		httperr.NotFound(w, "run not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run": map[string]any{
			"id":               runID.String(),
			"project_id":       projectID.String(),
			"commit_sha":       commitSha,
			"branch":           nullStr(branch),
			"ci_run_id":        nullStr(ciRunID),
			"github_pr_number": nullInt32(prNum),
			"status":           status,
			"started_at":       fmtTime(startedAt),
			"completed_at":     fmtTimePtr(completedAt),
			"total_tests":      total,
			"passed_tests":     passed,
			"failed_tests":     failed,
			"skipped_tests":    skipped,
		},
	})
}

func (h *CoverageHandler) ListRunTests(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.BadRequest(w, "invalid project_id")
		return
	}
	runID, err := parseUUID(chi.URLParam(r, "run_id"))
	if err != nil {
		httperr.BadRequest(w, "invalid run_id")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "unauthorized")
		return
	}
	if _, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID); err != nil {
		httperr.NotFound(w, "project not found")
		return
	}

	statusFilter := r.URL.Query().Get("status")
	var offset int64
	if c := r.URL.Query().Get("cursor"); c != "" {
		offset, _ = strconv.ParseInt(c, 10, 64)
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT t.id, t.name, t.suite, t.file_path, t.status,
		        t.started_at, t.completed_at, t.duration_ms,
		        COUNT(DISTINCT ce.line_start)::INT AS covered_lines
		   FROM tests t
		   LEFT JOIN coverage_events ce ON ce.test_id = t.id
		  WHERE t.run_id=$1
		    AND ($2='' OR t.status::TEXT=$2)
		  GROUP BY t.id
		  ORDER BY t.started_at
		  LIMIT 51 OFFSET $3`,
		pgconv.UUID(runID), statusFilter, offset,
	)
	if err != nil {
		httperr.Internal(w, err.Error())
		return
	}
	defer rows.Close()

	type testRow struct {
		ID, Name, Suite, FilePath, Status string
		StartedAt                         pgtype.Timestamptz
		CompletedAt                       pgtype.Timestamptz
		DurationMs                        *int32
		CoveredLines                      int32
	}

	tests := make([]map[string]any, 0, 50)
	for rows.Next() {
		var tr testRow
		var rawID pgtype.UUID
		if err := rows.Scan(&rawID, &tr.Name, &tr.Suite, &tr.FilePath, &tr.Status,
			&tr.StartedAt, &tr.CompletedAt, &tr.DurationMs, &tr.CoveredLines); err != nil {
			httperr.Internal(w, err.Error())
			return
		}
		tr.ID = pgconv.FromUUID(rawID).String()
		tests = append(tests, map[string]any{
			"id":            tr.ID,
			"run_id":        runID.String(),
			"name":          tr.Name,
			"suite":         tr.Suite,
			"file_path":     tr.FilePath,
			"status":        tr.Status,
			"started_at":    fmtTime(tr.StartedAt),
			"completed_at":  fmtTimePtr(tr.CompletedAt),
			"duration_ms":   nullInt32(tr.DurationMs),
			"covered_lines": tr.CoveredLines,
		})
	}

	var nextCursor *string
	if len(tests) == 51 {
		next := strconv.FormatInt(offset+50, 10)
		nextCursor = &next
		tests = tests[:50]
	}
	writeJSON(w, http.StatusOK, map[string]any{"tests": tests, "next_cursor": nextCursor})
}

// ── Gaps ──────────────────────────────────────────────────────────────────────

func (h *CoverageHandler) GetGaps(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.BadRequest(w, "invalid project_id")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "unauthorized")
		return
	}
	if _, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID); err != nil {
		httperr.NotFound(w, "project not found")
		return
	}

	pathFilter := r.URL.Query().Get("path")
	if len(pathFilter) > 500 {
		httperr.BadRequest(w, "path filter exceeds 500 characters")
		return
	}

	var minRisk float64
	if s := r.URL.Query().Get("min_risk"); s != "" {
		minRisk, _ = strconv.ParseFloat(s, 64)
	}

	var offset int64
	if c := r.URL.Query().Get("cursor"); c != "" {
		offset, _ = strconv.ParseInt(c, 10, 64)
	}

	ctx := r.Context()

	// Count total for pagination metadata.
	var total int64
	_ = h.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT file_path) FROM coverage_events
		  WHERE project_id=$1 AND hit_count=0
		    AND ($2='' OR file_path ILIKE '%'||$2||'%')`,
		pgconv.UUID(projectID), pathFilter,
	).Scan(&total)

	rows, err := h.pool.Query(ctx,
		`SELECT file_path,
		        MIN(line_start) AS line_start,
		        MAX(line_end)   AS line_end,
		        COUNT(DISTINCT line_start) AS uncovered_lines,
		        ROUND(
		            (COUNT(DISTINCT line_start)::NUMERIC /
		             NULLIF(MAX(line_end) - MIN(line_start) + 1, 0)) * 0.5
		            + 0.15
		            + LEAST(COUNT(DISTINCT line_start)::NUMERIC / 200, 0.2)
		        , 2) AS risk_score
		   FROM coverage_events
		  WHERE project_id=$1 AND hit_count=0
		    AND ($2='' OR file_path ILIKE '%'||$2||'%')
		  GROUP BY file_path
		  HAVING ($3=0 OR
		    ROUND(
		        (COUNT(DISTINCT line_start)::NUMERIC /
		         NULLIF(MAX(line_end) - MIN(line_start) + 1, 0)) * 0.5
		        + 0.15
		        + LEAST(COUNT(DISTINCT line_start)::NUMERIC / 200, 0.2)
		    , 2) >= $3)
		  ORDER BY risk_score DESC, uncovered_lines DESC
		  LIMIT 51 OFFSET $4`,
		pgconv.UUID(projectID), pathFilter, minRisk, offset,
	)
	if err != nil {
		httperr.Internal(w, err.Error())
		return
	}
	gaps := collectGaps(rows)

	var nextCursor *string
	if len(gaps) == 51 {
		next := strconv.FormatInt(offset+50, 10)
		nextCursor = &next
		gaps = gaps[:50]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"gaps":        gaps,
		"next_cursor": nextCursor,
		"total":       total,
	})
}

// ── File coverage ─────────────────────────────────────────────────────────────

func (h *CoverageHandler) GetFileCoverage(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.BadRequest(w, "invalid project_id")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "unauthorized")
		return
	}
	if _, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID); err != nil {
		httperr.NotFound(w, "project not found")
		return
	}

	// chi wildcard captures everything after the prefix including slashes.
	filePath := chi.URLParam(r, "*")
	if err := validateFilePath(filePath); err != nil {
		httperr.BadRequest(w, err.Error())
		return
	}

	commitSHA := r.URL.Query().Get("commit")
	if commitSHA == "" {
		httperr.BadRequest(w, "commit query param required")
		return
	}

	ctx := r.Context()

	// Resolve to a run for this project + commit.
	var runID pgtype.UUID
	err = h.pool.QueryRow(ctx,
		`SELECT id FROM runs
		  WHERE project_id=$1 AND commit_sha=$2 AND status='completed'
		  ORDER BY started_at DESC LIMIT 1`,
		pgconv.UUID(projectID), commitSHA,
	).Scan(&runID)
	if err != nil {
		httperr.NotFound(w, "no completed run found for this commit")
		return
	}

	rows, err := h.pool.Query(ctx,
		`SELECT ce.line_start AS line,
		        SUM(ce.hit_count)::INT AS hit_count,
		        ARRAY_AGG(DISTINCT t.id::TEXT) FILTER (WHERE t.id IS NOT NULL) AS test_ids
		   FROM coverage_events ce
		   LEFT JOIN tests t ON t.id = ce.test_id
		  WHERE ce.run_id=$1 AND ce.file_path=$2
		  GROUP BY ce.line_start
		  ORDER BY ce.line_start`,
		runID, filePath,
	)
	if err != nil {
		httperr.Internal(w, err.Error())
		return
	}
	defer rows.Close()

	type lineRow struct {
		Line     int32
		HitCount int32
		TestIDs  []string
	}
	lineRows := make([]map[string]any, 0, 256)
	for rows.Next() {
		var lr lineRow
		if err := rows.Scan(&lr.Line, &lr.HitCount, &lr.TestIDs); err != nil {
			httperr.Internal(w, err.Error())
			return
		}
		testIDs := lr.TestIDs
		if testIDs == nil {
			testIDs = []string{}
		}
		lineRows = append(lineRows, map[string]any{
			"line":      lr.Line,
			"hit_count": lr.HitCount,
			"test_ids":  testIDs,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"file_path":  filePath,
		"commit_sha": commitSHA,
		"lines":      lineRows,
	})
}

func (h *CoverageHandler) GetLineTests(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.BadRequest(w, "invalid project_id")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "unauthorized")
		return
	}
	if _, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID); err != nil {
		httperr.NotFound(w, "project not found")
		return
	}

	filePath := chi.URLParam(r, "path")
	if err := validateFilePath(filePath); err != nil {
		httperr.BadRequest(w, err.Error())
		return
	}
	lineNum, err := strconv.Atoi(chi.URLParam(r, "line"))
	if err != nil || lineNum < 1 {
		httperr.BadRequest(w, "invalid line number")
		return
	}
	commitSHA := r.URL.Query().Get("commit")
	if commitSHA == "" {
		httperr.BadRequest(w, "commit query param required")
		return
	}

	ctx := r.Context()

	var runID pgtype.UUID
	err = h.pool.QueryRow(ctx,
		`SELECT id FROM runs
		  WHERE project_id=$1 AND commit_sha=$2 AND status='completed'
		  ORDER BY started_at DESC LIMIT 1`,
		pgconv.UUID(projectID), commitSHA,
	).Scan(&runID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"tests": []any{}})
		return
	}

	rows, err := h.pool.Query(ctx,
		`SELECT DISTINCT t.id, t.name, t.suite
		   FROM coverage_events ce
		   JOIN tests t ON t.id = ce.test_id
		  WHERE ce.run_id=$1
		    AND ce.file_path=$2
		    AND $3 BETWEEN ce.line_start AND ce.line_end
		  ORDER BY t.name`,
		runID, filePath, lineNum,
	)
	if err != nil {
		httperr.Internal(w, err.Error())
		return
	}
	defer rows.Close()

	type testRef struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Suite string `json:"suite"`
	}
	tests := make([]testRef, 0)
	for rows.Next() {
		var rawID pgtype.UUID
		var tr testRef
		if err := rows.Scan(&rawID, &tr.Name, &tr.Suite); err != nil {
			httperr.Internal(w, err.Error())
			return
		}
		tr.ID = pgconv.FromUUID(rawID).String()
		tests = append(tests, tr)
	}

	writeJSON(w, http.StatusOK, map[string]any{"tests": tests})
}

// ── PR coverage ───────────────────────────────────────────────────────────────

func (h *CoverageHandler) GetPRCoverage(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseUUID(chi.URLParam(r, "project_id"))
	if err != nil {
		httperr.BadRequest(w, "invalid project_id")
		return
	}
	userID, ok := tenancy.UserFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, "unauthorized")
		return
	}
	if _, err := lookupProjectOrg(r.Context(), h.pool, projectID, userID); err != nil {
		httperr.NotFound(w, "project not found")
		return
	}

	prNum, err := strconv.Atoi(chi.URLParam(r, "pr_number"))
	if err != nil || prNum < 1 {
		httperr.BadRequest(w, "invalid pr_number")
		return
	}

	ctx := r.Context()

	// Find the latest run for this PR.
	var runID pgtype.UUID
	var headSHA, status string
	err = h.pool.QueryRow(ctx,
		`SELECT id, commit_sha, status FROM runs
		  WHERE project_id=$1 AND github_pr_number=$2
		  ORDER BY started_at DESC LIMIT 1`,
		pgconv.UUID(projectID), prNum,
	).Scan(&runID, &headSHA, &status)

	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"pr": map[string]any{
				"pr_number":     prNum,
				"head_sha":      "",
				"run_id":        nil,
				"pending":       true,
				"changed_files": []any{},
			},
		})
		return
	}

	pending := status != "completed"
	runIDStr := pgconv.FromUUID(runID).String()

	// Per-file coverage for this run.
	fileRows, err := h.pool.Query(ctx,
		`SELECT file_path,
		        COUNT(DISTINCT line_start) FILTER (WHERE hit_count > 0) AS covered,
		        COUNT(DISTINCT line_start) FILTER (WHERE hit_count = 0) AS uncovered
		   FROM coverage_events
		  WHERE run_id=$1
		  GROUP BY file_path
		  ORDER BY file_path`,
		runID,
	)
	if err != nil {
		httperr.Internal(w, err.Error())
		return
	}
	defer fileRows.Close()

	type fileEntry struct {
		FilePath       string `json:"file_path"`
		CoveredLines   int64  `json:"covered_lines"`
		UncoveredLines int64  `json:"uncovered_lines"`
		AddedLines     int64  `json:"added_lines"`
	}
	files := make([]fileEntry, 0)
	for fileRows.Next() {
		var fe fileEntry
		if err := fileRows.Scan(&fe.FilePath, &fe.CoveredLines, &fe.UncoveredLines); err != nil {
			httperr.Internal(w, err.Error())
			return
		}
		fe.AddedLines = fe.CoveredLines + fe.UncoveredLines
		files = append(files, fe)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"pr": map[string]any{
			"pr_number":     prNum,
			"head_sha":      headSHA,
			"run_id":        runIDStr,
			"pending":       pending,
			"changed_files": files,
		},
	})
}

// ── row collectors ────────────────────────────────────────────────────────────

func collectRuns(rows pgx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id pgtype.UUID
		var commitSha, status string
		var branch, ciRunID *string
		var prNum *int32
		var startedAt, completedAt pgtype.Timestamptz
		var total, passed, failed, skipped int32
		if err := rows.Scan(&id, &commitSha, &branch, &ciRunID, &prNum,
			&status, &startedAt, &completedAt,
			&total, &passed, &failed, &skipped); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{
			"id":               pgconv.FromUUID(id).String(),
			"commit_sha":       commitSha,
			"branch":           nullStr(branch),
			"ci_run_id":        nullStr(ciRunID),
			"github_pr_number": nullInt32(prNum),
			"status":           status,
			"started_at":       fmtTime(startedAt),
			"completed_at":     fmtTimePtr(completedAt),
			"total_tests":      total,
			"passed_tests":     passed,
			"failed_tests":     failed,
			"skipped_tests":    skipped,
		})
	}
	return result, rows.Err()
}

func collectGaps(rows pgx.Rows) []map[string]any {
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var filePath string
		var lineStart, lineEnd int32
		var uncoveredLines int64
		var riskScore float64
		if err := rows.Scan(&filePath, &lineStart, &lineEnd, &uncoveredLines, &riskScore); err != nil {
			continue
		}
		result = append(result, map[string]any{
			"file_path":       filePath,
			"function_name":   "",
			"line_start":      lineStart,
			"line_end":        lineEnd,
			"uncovered_lines": uncoveredLines,
			"risk_score":      riskScore,
		})
	}
	return result
}

func collectPRSummaries(rows pgx.Rows) []map[string]any {
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id pgtype.UUID
		var prNum *int32
		var commitSha, status string
		if err := rows.Scan(&id, &prNum, &commitSha, &status); err != nil {
			continue
		}
		runID := pgconv.FromUUID(id).String()
		result = append(result, map[string]any{
			"pr_number":       nullInt32(prNum),
			"title":           "",
			"head_sha":        commitSha,
			"covered_delta":   0,
			"uncovered_delta": 0,
			"run_id":          runID,
		})
	}
	return result
}

// ── pgtype helpers ────────────────────────────────────────────────────────────

func nilIfZero(u pgtype.UUID) any {
	if !u.Valid {
		return nil
	}
	return pgconv.FromUUID(u).String()
}

func nilIfTextEmpty(t pgtype.Text) any {
	if !t.Valid || t.String == "" {
		return nil
	}
	return t.String
}
