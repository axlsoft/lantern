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

-- name: ListRunsByProject :many
SELECT * FROM runs
WHERE project_id = $1
ORDER BY started_at DESC
LIMIT $2 OFFSET $3;

-- name: ListRunsByProjectAndPR :many
SELECT * FROM runs
WHERE project_id = $1 AND github_pr_number = $2
ORDER BY started_at DESC;

-- name: GetLatestCompletedRunByProject :one
SELECT * FROM runs
WHERE project_id = $1 AND status = 'completed'
ORDER BY started_at DESC
LIMIT 1;

-- name: ListTestsByRunPaginated :many
SELECT t.*,
       COUNT(DISTINCT ce.line_start)::INT AS covered_lines
FROM tests t
LEFT JOIN coverage_events ce ON ce.test_id = t.id
WHERE t.run_id = $1
  AND ($2::TEXT = '' OR t.status::TEXT = $2)
GROUP BY t.id
ORDER BY t.started_at
LIMIT $3 OFFSET $4;

-- name: CountTestsByRun :one
SELECT COUNT(*)::BIGINT FROM tests
WHERE run_id = $1
  AND ($2::TEXT = '' OR status::TEXT = $2);
