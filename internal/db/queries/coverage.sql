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

-- name: GetDistinctFilesByRun :many
SELECT DISTINCT file_path FROM coverage_events WHERE run_id = $1 ORDER BY file_path;

-- name: GetCoveredLinesByRunAndFile :many
SELECT DISTINCT line_start, SUM(hit_count)::INT AS total_hits
FROM coverage_events
WHERE run_id = $1 AND file_path = $2
GROUP BY line_start
ORDER BY line_start;

-- name: GetTestsForLine :many
SELECT DISTINCT t.id, t.name, t.suite
FROM coverage_events ce
JOIN tests t ON t.id = ce.test_id
WHERE ce.run_id = $1
  AND ce.file_path = $2
  AND $3 BETWEEN ce.line_start AND ce.line_end
ORDER BY t.name;

-- name: GetUncoveredLineCountByProject :one
SELECT COUNT(DISTINCT (file_path, line_start))::BIGINT AS uncovered_count
FROM (
    -- Lines that appear in a file version but have no coverage event in the latest run
    SELECT fv.file_path, generate_series(1, fv.line_count) AS line_start
    FROM file_versions fv
    WHERE fv.project_id = $1
    EXCEPT
    SELECT ce.file_path, ce.line_start
    FROM coverage_events ce
    WHERE ce.project_id = $1
) sub;

-- name: GetCoverageGapsByProject :many
SELECT
    ce_outer.file_path,
    MIN(ce_outer.line_start) AS line_start,
    MAX(ce_outer.line_end)   AS line_end,
    COUNT(DISTINCT ce_outer.line_start) AS uncovered_lines,
    -- Risk score: 0.5 * (uncovered_ratio) + 0.3 * 0.5 + 0.2 * size_factor
    -- Simplified MVP formula; not joined to git history here.
    ROUND(
        (COUNT(DISTINCT ce_outer.line_start)::NUMERIC
         / NULLIF((MAX(ce_outer.line_end) - MIN(ce_outer.line_start) + 1), 0))
        * 0.5
        + 0.15
        + LEAST(COUNT(DISTINCT ce_outer.line_start)::NUMERIC / 200, 0.2)
    , 2) AS risk_score,
    '' AS function_name
FROM coverage_events ce_outer
WHERE ce_outer.project_id = $1
  AND ce_outer.hit_count = 0
  AND ($2::TEXT = '' OR ce_outer.file_path ILIKE '%' || $2 || '%')
GROUP BY ce_outer.file_path
HAVING
    ($3::NUMERIC = 0 OR
     ROUND(
        (COUNT(DISTINCT ce_outer.line_start)::NUMERIC
         / NULLIF((MAX(ce_outer.line_end) - MIN(ce_outer.line_start) + 1), 0))
        * 0.5 + 0.15
        + LEAST(COUNT(DISTINCT ce_outer.line_start)::NUMERIC / 200, 0.2)
     , 2) >= $3::NUMERIC)
ORDER BY risk_score DESC, uncovered_lines DESC
LIMIT $4 OFFSET $5;

-- name: CountCoverageGapsByProject :one
SELECT COUNT(DISTINCT file_path)::BIGINT AS total
FROM coverage_events
WHERE project_id = $1
  AND hit_count = 0
  AND ($2::TEXT = '' OR file_path ILIKE '%' || $2 || '%');

-- name: GetCoverageTestsByRunAndFileLine :many
SELECT DISTINCT t.id, t.name, t.suite
FROM coverage_events ce
JOIN tests t ON t.id = ce.test_id
WHERE ce.run_id = $1
  AND ce.file_path = $2
  AND $3 BETWEEN ce.line_start AND ce.line_end
ORDER BY t.name;

-- name: GetCoverageSummaryByProject :one
SELECT
    COUNT(DISTINCT ce.file_path || ':' || ce.line_start) FILTER (WHERE ce.hit_count > 0) AS covered_lines,
    COUNT(DISTINCT ce.file_path || ':' || ce.line_start) FILTER (WHERE ce.hit_count = 0) AS uncovered_lines,
    COUNT(DISTINCT ce.file_path || ':' || ce.line_start) AS total_lines
FROM coverage_events ce
JOIN runs r ON r.id = ce.run_id
WHERE ce.project_id = $1
  AND r.id = (
      SELECT id FROM runs
      WHERE project_id = $1 AND status = 'completed'
      ORDER BY started_at DESC
      LIMIT 1
  );

-- name: GetCoverageByRunFileForUI :many
SELECT
    ce.line_start AS line,
    SUM(ce.hit_count)::INT AS hit_count,
    ARRAY_AGG(DISTINCT t.id::TEXT) FILTER (WHERE t.id IS NOT NULL) AS test_ids
FROM coverage_events ce
LEFT JOIN tests t ON t.id = ce.test_id
WHERE ce.run_id = $1 AND ce.file_path = $2
GROUP BY ce.line_start
ORDER BY ce.line_start;
