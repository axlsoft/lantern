-- Migration 004: SECURITY DEFINER helper functions for management API lookups.
--
-- The management API needs to resolve a resource's organization_id before it can
-- open an org-scoped transaction. Because teams/projects are RLS-protected, a
-- direct query from lantern_app returns no rows without a prior org context.
-- These SECURITY DEFINER functions run as the migration owner (bypassing RLS)
-- and return the org_id only if the requesting user is a member of that org.

CREATE OR REPLACE FUNCTION get_team_org_id(p_team_id UUID, p_user_id UUID)
RETURNS UUID
LANGUAGE sql
STABLE
SECURITY DEFINER
AS $$
    SELECT t.organization_id
    FROM teams t
    INNER JOIN organization_memberships om
        ON om.organization_id = t.organization_id
    WHERE t.id = p_team_id
      AND om.user_id = p_user_id
      AND t.deleted_at IS NULL
    LIMIT 1;
$$;

CREATE OR REPLACE FUNCTION get_project_org_id(p_project_id UUID, p_user_id UUID)
RETURNS UUID
LANGUAGE sql
STABLE
SECURITY DEFINER
AS $$
    SELECT p.organization_id
    FROM projects p
    INNER JOIN organization_memberships om
        ON om.organization_id = p.organization_id
    WHERE p.id = p_project_id
      AND om.user_id = p_user_id
      AND p.deleted_at IS NULL
    LIMIT 1;
$$;

GRANT EXECUTE ON FUNCTION get_team_org_id(UUID, UUID)    TO lantern_app;
GRANT EXECUTE ON FUNCTION get_project_org_id(UUID, UUID) TO lantern_app;

-- For the API key authentication middleware: resolves a key by prefix without
-- requiring an org context. The middleware authenticates the key BEFORE knowing
-- which org it belongs to, so it cannot pre-set lantern.current_organization_id.
CREATE OR REPLACE FUNCTION get_api_key_by_prefix(p_prefix TEXT)
RETURNS SETOF api_keys
LANGUAGE sql
STABLE
SECURITY DEFINER
AS $$
    SELECT * FROM api_keys
    WHERE key_prefix = p_prefix
      AND revoked_at IS NULL
    LIMIT 1;
$$;

-- For best-effort last-used tracking in the API key middleware goroutine.
CREATE OR REPLACE FUNCTION touch_api_key_last_used(p_key_id UUID)
RETURNS void
LANGUAGE sql
SECURITY DEFINER
AS $$
    UPDATE api_keys SET last_used_at = now() WHERE id = p_key_id;
$$;

GRANT EXECUTE ON FUNCTION get_api_key_by_prefix(TEXT)   TO lantern_app;
GRANT EXECUTE ON FUNCTION touch_api_key_last_used(UUID)  TO lantern_app;
