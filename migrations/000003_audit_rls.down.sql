-- Remove RLS policies first, then the table.
DROP POLICY IF EXISTS tenant_isolation ON audit_log;
DROP POLICY IF EXISTS tenant_isolation ON file_versions;
DROP POLICY IF EXISTS tenant_isolation ON coverage_events;
DROP POLICY IF EXISTS tenant_isolation ON tests;
DROP POLICY IF EXISTS tenant_isolation ON runs;
DROP POLICY IF EXISTS tenant_isolation ON api_keys;
DROP POLICY IF EXISTS tenant_isolation ON teams;
DROP POLICY IF EXISTS tenant_isolation ON projects;

ALTER TABLE projects        DISABLE ROW LEVEL SECURITY;
ALTER TABLE teams           DISABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys        DISABLE ROW LEVEL SECURITY;
ALTER TABLE runs            DISABLE ROW LEVEL SECURITY;
ALTER TABLE tests           DISABLE ROW LEVEL SECURITY;
ALTER TABLE coverage_events DISABLE ROW LEVEL SECURITY;
ALTER TABLE file_versions   DISABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log       DISABLE ROW LEVEL SECURITY;

DROP TABLE IF EXISTS audit_log;
