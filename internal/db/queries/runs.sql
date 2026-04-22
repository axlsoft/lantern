-- name: CreateRun :one
INSERT INTO runs (project_id, organization_id, commit_sha, branch, ci_run_id, github_pr_number, attribution_mode)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetRun :one
SELECT * FROM runs WHERE id = $1 LIMIT 1;

-- name: UpdateRunStatus :one
UPDATE runs
SET status = $2,
    completed_at = CASE WHEN $2 = ANY(ARRAY['completed','failed','aborted']::run_status[]) THEN now() ELSE completed_at END,
    total_tests = $3,
    passed_tests = $4,
    failed_tests = $5,
    skipped_tests = $6
WHERE id = $1
RETURNING *;

-- name: CreateTest :one
INSERT INTO tests (run_id, project_id, organization_id, test_external_id, name, suite, file_path)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetTestByExternalID :one
SELECT * FROM tests WHERE run_id = $1 AND test_external_id = $2 LIMIT 1;

-- name: UpdateTestStatus :one
UPDATE tests
SET status = $2,
    completed_at = now(),
    duration_ms = $3
WHERE id = $1
RETURNING *;

-- name: ListTestsByRun :many
SELECT * FROM tests WHERE run_id = $1 ORDER BY started_at;
