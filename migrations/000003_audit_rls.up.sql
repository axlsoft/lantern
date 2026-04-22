-- Migration 003: Audit log and row-level security.

-- ── Audit Log ─────────────────────────────────────────────────────────────────

CREATE TABLE audit_log (
    id               BIGSERIAL   PRIMARY KEY,
    organization_id  UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    actor_user_id    UUID        REFERENCES users(id) ON DELETE SET NULL,
    actor_api_key_id UUID        REFERENCES api_keys(id) ON DELETE SET NULL,
    action           TEXT        NOT NULL,
    resource_type    TEXT        NOT NULL,
    resource_id      TEXT        NOT NULL,
    metadata         JSONB       NOT NULL DEFAULT '{}',
    ip_address       INET,
    user_agent       TEXT,
    occurred_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_organization_id ON audit_log(organization_id);
CREATE INDEX idx_audit_log_occurred_at     ON audit_log(occurred_at DESC);
CREATE INDEX idx_audit_log_actor_user_id   ON audit_log(actor_user_id) WHERE actor_user_id IS NOT NULL;

-- ── Row-Level Security ────────────────────────────────────────────────────────
-- All tenant-scoped tables check lantern.current_organization_id which is SET LOCAL
-- at the start of every authenticated transaction by the tenant context middleware.
-- Tables without a direct organization_id column carry a denormalized copy (added
-- in migrations 001/002) specifically to make these policies a simple equality check.

ALTER TABLE projects        ENABLE ROW LEVEL SECURITY;
ALTER TABLE teams           ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys        ENABLE ROW LEVEL SECURITY;
ALTER TABLE runs            ENABLE ROW LEVEL SECURITY;
ALTER TABLE tests           ENABLE ROW LEVEL SECURITY;
ALTER TABLE coverage_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_versions   ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log       ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON projects
    USING (organization_id = current_setting('lantern.current_organization_id', true)::uuid);

CREATE POLICY tenant_isolation ON teams
    USING (organization_id = current_setting('lantern.current_organization_id', true)::uuid);

CREATE POLICY tenant_isolation ON api_keys
    USING (organization_id = current_setting('lantern.current_organization_id', true)::uuid);

CREATE POLICY tenant_isolation ON runs
    USING (organization_id = current_setting('lantern.current_organization_id', true)::uuid);

CREATE POLICY tenant_isolation ON tests
    USING (organization_id = current_setting('lantern.current_organization_id', true)::uuid);

CREATE POLICY tenant_isolation ON coverage_events
    USING (organization_id = current_setting('lantern.current_organization_id', true)::uuid);

CREATE POLICY tenant_isolation ON file_versions
    USING (organization_id = current_setting('lantern.current_organization_id', true)::uuid);

CREATE POLICY tenant_isolation ON audit_log
    USING (organization_id = current_setting('lantern.current_organization_id', true)::uuid);

-- NOTE: lantern_app must be created by ops with NOBYPASSRLS (the default for non-superusers).
-- Role attribute management is not done in application migrations — it requires CREATEROLE privilege.
