-- Run this as the migration role (or superuser) after all migrations have been applied.
-- Re-run whenever new tables are added.
--
-- Usage:
--   psql $MIGRATE_URL -f migrations/grants.sql

GRANT USAGE ON SCHEMA public TO lantern_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO lantern_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO lantern_app;

-- Future tables/sequences created by lantern_migrate are automatically accessible.
ALTER DEFAULT PRIVILEGES FOR ROLE lantern_migrate IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO lantern_app;
ALTER DEFAULT PRIVILEGES FOR ROLE lantern_migrate IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO lantern_app;
