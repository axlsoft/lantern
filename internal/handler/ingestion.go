package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	lanternv1 "github.com/axlsoft/lantern/gen/proto/lantern/v1"
	"github.com/axlsoft/lantern/internal/config"
	"github.com/axlsoft/lantern/internal/db/generated"
	"github.com/axlsoft/lantern/internal/httperr"
	"github.com/axlsoft/lantern/internal/pgconv"
	"github.com/axlsoft/lantern/internal/tenancy"
)

const supportedSchemaVersion = "1"

// IngestionHandler handles coverage and run lifecycle ingestion endpoints.
type IngestionHandler struct {
	pool *pgxpool.Pool
	q    *generated.Queries
	cfg  *config.Config
}

// NewIngestionHandler constructs an IngestionHandler.
func NewIngestionHandler(pool *pgxpool.Pool, cfg *config.Config) *IngestionHandler {
	return &IngestionHandler{pool: pool, q: generated.New(pool), cfg: cfg}
}

// ── POST /v1/coverage ─────────────────────────────────────────────────────────

func (h *IngestionHandler) IngestCoverage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxRequestBodyBytes)

	batch, err := decodeCoverageBatch(r)
	if err != nil {
		httperr.BadRequest(w, err.Error())
		return
	}

	if batch.Resource == nil {
		httperr.BadRequest(w, "resource is required")
		return
	}
	if batch.Resource.SchemaVersion != supportedSchemaVersion {
		httperr.BadRequest(w, fmt.Sprintf("unsupported schema_version %q; supported: %q", batch.Resource.SchemaVersion, supportedSchemaVersion))
		return
	}

	orgID, err := tenancy.OrgFromContext(r.Context())
	if err != nil {
		httperr.Unauthorized(w, "authentication required")
		return
	}

	projectID, err := parseUUID(batch.Resource.ProjectId)
	if err != nil {
		httperr.BadRequest(w, "invalid project_id in resource")
		return
	}

	runID, err := parseUUID(batch.Resource.RunId)
	if err != nil {
		httperr.BadRequest(w, "invalid run_id in resource")
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	project, err := q.GetProject(r.Context(), pgconv.UUID(projectID))
	if err != nil || pgconv.FromUUID(project.OrganizationID) != orgID {
		httperr.Forbidden(w, "project not accessible with this API key")
		return
	}

	run, err := q.GetRun(r.Context(), pgconv.UUID(runID))
	if err != nil || pgconv.FromUUID(run.ProjectID) != projectID {
		httperr.BadRequest(w, "run not found in this project")
		return
	}

	// Idempotency: skip if batch_id already processed.
	if batch.BatchId != "" {
		exists, err := q.BatchIDExists(r.Context(), generated.BatchIDExistsParams{
			RunID:   pgconv.UUID(runID),
			BatchID: batch.BatchId,
		})
		if err == nil && exists {
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"message": "batch already processed"}})
			return
		}
	}

	if len(batch.Events) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"inserted": 0}})
		return
	}

	var inserted int64
	for _, ev := range batch.Events {
		testID := pgconv.NullUUID(nil)
		if ev.TestId != "" {
			if tid, err := parseUUID(ev.TestId); err == nil {
				testID = pgconv.NullUUIDFromUUID(tid)
			}
		}
		var workerID *string
		if ev.WorkerId != "" {
			s := ev.WorkerId
			workerID = &s
		}
		n, err := q.InsertCoverageEvent(r.Context(), generated.InsertCoverageEventParams{
			RunID:          pgconv.UUID(runID),
			TestID:         testID,
			ProjectID:      pgconv.UUID(projectID),
			OrganizationID: project.OrganizationID,
			BatchID:        batch.BatchId,
			FilePath:       ev.FilePath,
			LineStart:      ev.LineStart,
			LineEnd:        ev.LineEnd,
			HitCount:       ev.HitCount,
			WorkerID:       workerID,
		})
		if err != nil {
			httperr.Internal(w, "could not insert coverage events")
			return
		}
		inserted += n
	}

	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"inserted": inserted}})
}

// ── POST /v1/runs ─────────────────────────────────────────────────────────────

type createRunRequest struct {
	ProjectID       string `json:"project_id"`
	CommitSha       string `json:"commit_sha"`
	Branch          string `json:"branch"`
	CiRunID         string `json:"ci_run_id"`
	GithubPRNumber  *int32 `json:"github_pr_number"`
	AttributionMode string `json:"attribution_mode"`
}

func (h *IngestionHandler) CreateRun(w http.ResponseWriter, r *http.Request) {
	var req createRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid JSON body")
		return
	}
	if req.ProjectID == "" || req.CommitSha == "" {
		httperr.BadRequest(w, "project_id and commit_sha are required")
		return
	}

	orgID, err := tenancy.OrgFromContext(r.Context())
	if err != nil {
		httperr.Unauthorized(w, "authentication required")
		return
	}

	projectID, err := parseUUID(req.ProjectID)
	if err != nil {
		httperr.BadRequest(w, "invalid project_id")
		return
	}

	if req.AttributionMode == "" {
		req.AttributionMode = "serialized"
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	project, err := q.GetProject(r.Context(), pgconv.UUID(projectID))
	if err != nil || pgconv.FromUUID(project.OrganizationID) != orgID {
		httperr.Forbidden(w, "project not accessible with this API key")
		return
	}

	var branch *string
	if req.Branch != "" {
		branch = &req.Branch
	}
	var ciRunID *string
	if req.CiRunID != "" {
		ciRunID = &req.CiRunID
	}

	run, err := q.CreateRun(r.Context(), generated.CreateRunParams{
		ProjectID:       pgconv.UUID(projectID),
		OrganizationID:  project.OrganizationID,
		CommitSha:       req.CommitSha,
		Branch:          branch,
		CiRunID:         ciRunID,
		GithubPrNumber:  req.GithubPRNumber,
		AttributionMode: req.AttributionMode,
	})
	if err != nil {
		httperr.Internal(w, "could not create run")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": run})
}

// ── PATCH /v1/runs/:run_id ────────────────────────────────────────────────────

type updateRunRequest struct {
	Status       generated.RunStatus `json:"status"`
	TotalTests   int32               `json:"total_tests"`
	PassedTests  int32               `json:"passed_tests"`
	FailedTests  int32               `json:"failed_tests"`
	SkippedTests int32               `json:"skipped_tests"`
}

func (h *IngestionHandler) UpdateRun(w http.ResponseWriter, r *http.Request) {
	runID, err := parseUUID(chi.URLParam(r, "run_id"))
	if err != nil {
		httperr.NotFound(w, "run not found")
		return
	}

	orgID, err := tenancy.OrgFromContext(r.Context())
	if err != nil {
		httperr.Unauthorized(w, "authentication required")
		return
	}

	var req updateRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid JSON body")
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	run, err := q.GetRun(r.Context(), pgconv.UUID(runID))
	if err != nil {
		httperr.NotFound(w, "run not found")
		return
	}
	if pgconv.FromUUID(run.OrganizationID) != orgID {
		httperr.Forbidden(w, "run not accessible")
		return
	}

	updated, err := q.UpdateRunStatus(r.Context(), generated.UpdateRunStatusParams{
		ID:           pgconv.UUID(runID),
		Status:       req.Status,
		TotalTests:   req.TotalTests,
		PassedTests:  req.PassedTests,
		FailedTests:  req.FailedTests,
		SkippedTests: req.SkippedTests,
	})
	if err != nil {
		httperr.Internal(w, "could not update run")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// ── POST /v1/runs/:run_id/tests ───────────────────────────────────────────────

type registerTestsRequest struct {
	Tests []struct {
		TestExternalID string `json:"test_external_id"`
		Name           string `json:"name"`
		Suite          string `json:"suite"`
		FilePath       string `json:"file_path"`
	} `json:"tests"`
}

func (h *IngestionHandler) RegisterTests(w http.ResponseWriter, r *http.Request) {
	runID, err := parseUUID(chi.URLParam(r, "run_id"))
	if err != nil {
		httperr.NotFound(w, "run not found")
		return
	}

	orgID, err := tenancy.OrgFromContext(r.Context())
	if err != nil {
		httperr.Unauthorized(w, "authentication required")
		return
	}

	var req registerTestsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid JSON body")
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	run, err := q.GetRun(r.Context(), pgconv.UUID(runID))
	if err != nil {
		httperr.NotFound(w, "run not found")
		return
	}
	if pgconv.FromUUID(run.OrganizationID) != orgID {
		httperr.Forbidden(w, "run not accessible")
		return
	}

	out := make([]generated.Test, 0, len(req.Tests))
	for _, t := range req.Tests {
		test, err := q.CreateTest(r.Context(), generated.CreateTestParams{
			RunID:          pgconv.UUID(runID),
			ProjectID:      run.ProjectID,
			OrganizationID: run.OrganizationID,
			TestExternalID: t.TestExternalID,
			Name:           t.Name,
			Suite:          t.Suite,
			FilePath:       t.FilePath,
		})
		if err != nil {
			httperr.Internal(w, "could not register test")
			return
		}
		out = append(out, test)
	}

	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": out})
}

// ── PATCH /v1/runs/:run_id/tests/:test_id ─────────────────────────────────────

type updateTestRequest struct {
	Status     generated.TestStatus `json:"status"`
	DurationMs *int32               `json:"duration_ms"`
}

func (h *IngestionHandler) UpdateTest(w http.ResponseWriter, r *http.Request) {
	testID, err := parseUUID(chi.URLParam(r, "test_id"))
	if err != nil {
		httperr.NotFound(w, "test not found")
		return
	}

	orgID, err := tenancy.OrgFromContext(r.Context())
	if err != nil {
		httperr.Unauthorized(w, "authentication required")
		return
	}

	var req updateTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, "invalid JSON body")
		return
	}

	tx, q, err := beginOrgTx(r.Context(), h.pool, orgID)
	if err != nil {
		httperr.Internal(w, "database error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	updated, err := q.UpdateTestStatus(r.Context(), generated.UpdateTestStatusParams{
		ID:         pgconv.UUID(testID),
		Status:     req.Status,
		DurationMs: req.DurationMs,
	})
	if err != nil {
		httperr.Internal(w, "could not update test")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		httperr.Internal(w, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": updated})
}

// ── Content-type helpers ──────────────────────────────────────────────────────

func decodeCoverageBatch(r *http.Request) (*lanternv1.CoverageBatch, error) {
	ct := r.Header.Get("Content-Type")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var batch lanternv1.CoverageBatch
	switch {
	case strings.Contains(ct, "application/x-protobuf"), strings.Contains(ct, "application/protobuf"):
		if err := proto.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("decode protobuf: %w", err)
		}
	case strings.Contains(ct, "application/json"):
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("decode JSON: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported Content-Type %q; use application/x-protobuf or application/json", ct)
	}

	return &batch, nil
}
