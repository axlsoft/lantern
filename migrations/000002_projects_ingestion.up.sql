-- Migration 002: Projects, API keys, runs, tests, coverage events, and file versions.

-- ── Projects ──────────────────────────────────────────────────────────────────

CREATE TABLE projects (
    id                      UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id         UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    team_id                 UUID        REFERENCES teams(id) ON DELETE SET NULL,
    name                    TEXT        NOT NULL,
    slug                    TEXT        NOT NULL,
    default_branch          TEXT        NOT NULL DEFAULT 'main',
    github_repo_full_name   TEXT,
    deleted_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);

CREATE INDEX idx_projects_organization_id ON projects(organization_id);
CREATE INDEX idx_projects_team_id ON projects(team_id);

-- ── API Keys ──────────────────────────────────────────────────────────────────

CREATE TABLE api_keys (
    id                  UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id          UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    organization_id     UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                TEXT        NOT NULL,
    key_hash            TEXT        NOT NULL UNIQUE,
    key_prefix          TEXT        NOT NULL,
    created_by_user_id  UUID        NOT NULL REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at        TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_project_id ON api_keys(project_id);
CREATE INDEX idx_api_keys_organization_id ON api_keys(organization_id);
-- Lookup by prefix for the two-step hash verification.
CREATE INDEX idx_api_keys_key_prefix ON api_keys(key_prefix) WHERE revoked_at IS NULL;

-- ── Runs ──────────────────────────────────────────────────────────────────────

CREATE TYPE run_status AS ENUM ('in_progress', 'completed', 'failed', 'aborted');

CREATE TABLE runs (
    id               UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id       UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    organization_id  UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    commit_sha       TEXT        NOT NULL,
    branch           TEXT,
    ci_run_id        TEXT,
    github_pr_number INT,
    status           run_status  NOT NULL DEFAULT 'in_progress',
    attribution_mode TEXT        NOT NULL DEFAULT 'serialized',
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at     TIMESTAMPTZ,
    total_tests      INT         NOT NULL DEFAULT 0,
    passed_tests     INT         NOT NULL DEFAULT 0,
    failed_tests     INT         NOT NULL DEFAULT 0,
    skipped_tests    INT         NOT NULL DEFAULT 0
);

CREATE INDEX idx_runs_project_id ON runs(project_id);
CREATE INDEX idx_runs_organization_id ON runs(organization_id);
CREATE INDEX idx_runs_status ON runs(status) WHERE status = 'in_progress';

-- ── Tests ─────────────────────────────────────────────────────────────────────

CREATE TYPE test_status AS ENUM ('passed', 'failed', 'skipped', 'timed_out');

CREATE TABLE tests (
    id               UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    run_id           UUID        NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    project_id       UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    organization_id  UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    test_external_id TEXT        NOT NULL,
    name             TEXT        NOT NULL,
    suite            TEXT        NOT NULL DEFAULT '',
    file_path        TEXT        NOT NULL DEFAULT '',
    status           test_status NOT NULL DEFAULT 'passed',
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at     TIMESTAMPTZ,
    duration_ms      INT
);

CREATE INDEX idx_tests_run_id ON tests(run_id);
CREATE INDEX idx_tests_project_id ON tests(project_id);
CREATE INDEX idx_tests_organization_id ON tests(organization_id);

-- ── Coverage Events ───────────────────────────────────────────────────────────

-- BIGSERIAL PK for index efficiency on the high-volume table; not UUID.
CREATE TABLE coverage_events (
    id               BIGSERIAL   PRIMARY KEY,
    run_id           UUID        NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    test_id          UUID        REFERENCES tests(id) ON DELETE SET NULL,
    project_id       UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    organization_id  UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    batch_id         TEXT        NOT NULL DEFAULT '',
    file_path        TEXT        NOT NULL,
    line_start       INT         NOT NULL,
    line_end         INT         NOT NULL,
    hit_count        INT         NOT NULL DEFAULT 1,
    worker_id        TEXT,
    recorded_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_coverage_events_project_run_file
    ON coverage_events(project_id, run_id, file_path);
CREATE INDEX idx_coverage_events_organization_id
    ON coverage_events(organization_id);
-- Unique constraint for idempotent batch ingestion.
CREATE UNIQUE INDEX idx_coverage_events_batch_dedup
    ON coverage_events(run_id, batch_id, file_path, line_start, line_end)
    WHERE batch_id <> '';

-- ── File Versions ─────────────────────────────────────────────────────────────

CREATE TABLE file_versions (
    id              UUID    PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id      UUID    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    organization_id UUID    NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    commit_sha      TEXT    NOT NULL,
    file_path       TEXT    NOT NULL,
    content_hash    TEXT    NOT NULL,
    line_count      INT     NOT NULL,
    UNIQUE (project_id, commit_sha, file_path)
);

CREATE INDEX idx_file_versions_project_id ON file_versions(project_id);
CREATE INDEX idx_file_versions_organization_id ON file_versions(organization_id);
