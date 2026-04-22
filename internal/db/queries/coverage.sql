-- name: InsertCoverageEvent :execrows
INSERT INTO coverage_events (
    run_id, test_id, project_id, organization_id,
    batch_id, file_path, line_start, line_end, hit_count, worker_id
) VALUES (
    @run_id, @test_id, @project_id, @organization_id,
    @batch_id, @file_path, @line_start, @line_end, @hit_count, @worker_id
) ON CONFLICT DO NOTHING;

-- name: BatchIDExists :one
SELECT EXISTS (
    SELECT 1 FROM coverage_events WHERE run_id = $1 AND batch_id = $2
) AS exists;

-- name: GetCoverageByRunAndFile :many
SELECT * FROM coverage_events
WHERE run_id = $1 AND file_path = $2
ORDER BY line_start;
