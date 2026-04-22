# Database Setup

This document covers everything needed to provision Postgres for Lantern development. It explains the *why* behind each step so you can adapt it when the environment differs.

---

## TL;DR

With Docker available:
```bash
docker compose -f deploy/docker/compose.dev.yml up -d
./scripts/setup-db.sh local
```

Without Docker (remote Postgres via SSH):
```bash
SSH_HOST=user@db01.example.com ./scripts/setup-db.sh tunnel
```

---

## Prerequisites

| Tool | Install |
|------|---------|
| `psql` | `brew install libpq` (macOS) or install with Postgres |
| `migrate` | `go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest` |
| `sqlc` | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest` (only needed if editing queries) |

---

## Option A: Docker (recommended for development)

The Compose file at `deploy/docker/compose.dev.yml` starts Postgres 16 on port 5432 and MailHog on ports 1025/8025.

```bash
docker compose -f deploy/docker/compose.dev.yml up -d
./scripts/setup-db.sh local
```

The script uses the `postgres` superuser (password `postgres`) to create roles, the database, and run migrations. Skip ahead to [Running Migrations](#running-migrations) if you prefer to do it manually.

---

## Option B: Remote Postgres via SSH Tunnel

When Docker is unavailable, forward a remote Postgres port to localhost using an SSH tunnel.

### 1. Open the tunnel

```bash
ssh -N -L 5432:127.0.0.1:5432 user@db01.example.com
```

Flags:
- `-N` — don't execute a remote command; just forward ports
- `-L 5432:127.0.0.1:5432` — local port 5432 → remote's loopback port 5432

Leave this running in a terminal, or use `-f` to background it. To background and track the PID:
```bash
ssh -f -N -L 5432:127.0.0.1:5432 user@db01.example.com
echo $!  # not reliable for -f; use: pgrep -n -f "ssh.*-L 5432"
```

The automated script (`./scripts/setup-db.sh tunnel`) opens and closes the tunnel automatically.

### 2. Verify the tunnel

```bash
psql "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable" -c "SELECT version();"
```

If this fails, the tunnel isn't forwarding correctly. Common causes:
- The remote Postgres is listening on `0.0.0.0` instead of `127.0.0.1` — adjust the tunnel target
- A local Postgres is already on port 5432 — use a different local port (e.g., `-L 5433:...`)
- The SSH server's `AllowTcpForwarding` is disabled

---

## Role and Database Provisioning

Connect as a Postgres superuser and run the following. The script does this automatically; this section explains each decision.

### Create the roles

```sql
-- Migration owner: creates and owns all schema objects.
-- CREATEDB is needed so lantern_migrate can create the database.
CREATE ROLE lantern_migrate
    WITH LOGIN PASSWORD 'changeme_migrate'
    CREATEDB NOCREATEROLE NOSUPERUSER NOREPLICATION NOBYPASSRLS;

-- Application role: used by the running collector process.
-- NOBYPASSRLS is the default for non-superusers but we state it explicitly.
-- The entire RLS security model breaks if this role has BYPASSRLS.
CREATE ROLE lantern_app
    WITH LOGIN PASSWORD 'changeme_app'
    NOCREATEDB NOCREATEROLE NOSUPERUSER NOREPLICATION NOBYPASSRLS;
```

**Why two roles?**

| | `lantern_migrate` | `lantern_app` |
|---|---|---|
| Owns tables/sequences | ✓ | — |
| Runs migrations | ✓ | — |
| Used by application | — | ✓ |
| Subject to RLS | — (owner, BYPASSRLS by default) | ✓ (enforced) |

Separating the migration role from the application role means a compromised application process cannot drop tables, alter schema, or bypass row-level security.

**NOBYPASSRLS is critical.** Postgres grants BYPASSRLS to superusers automatically. For `lantern_app`, `NOBYPASSRLS` is the default but worth stating explicitly. If this role ever gains BYPASSRLS (e.g., by accident during a `ALTER ROLE ... SUPERUSER`), cross-tenant data becomes visible to the application.

### Create the database

```sql
-- Must run outside a transaction block.
CREATE DATABASE lantern OWNER lantern_migrate;

-- Let the app role connect.
GRANT CONNECT ON DATABASE lantern TO lantern_app;
```

`OWNER lantern_migrate` means the migration role owns the database and all objects created within it. The application role only gets `CONNECT`.

### Enable the uuid-ossp extension

```sql
\c lantern  -- connect to the new database as superuser
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
```

This must be done as superuser. The extension provides `uuid_generate_v4()`, used as the default for UUID primary keys throughout the schema.

---

## Running Migrations

```bash
migrate \
  -path migrations \
  -database "postgres://lantern_migrate:changeme_migrate@localhost:5432/lantern?sslmode=disable" \
  up
```

Migrations are in `migrations/` with the `golang-migrate` naming convention (`000001_auth_tenancy.up.sql`, etc.). They are applied in order and tracked in the `schema_migrations` table.

To check current migration state:
```bash
migrate \
  -path migrations \
  -database "postgres://lantern_migrate:..." \
  version
```

### Migration summary

| File | What it creates |
|------|----------------|
| `000001_auth_tenancy` | Users, organizations, teams, sessions, email verification, password reset, org invites, org/team membership |
| `000002_projects_ingestion` | Projects, API keys, runs, tests, coverage events, file versions |
| `000003_audit_rls` | Audit log table; enables RLS on 8 tenant-scoped tables |
| `000004_management_functions` | 4 SECURITY DEFINER functions (see below) |

---

## Applying Table Grants

Migrations run as `lantern_migrate`, which owns all tables. A separate grants step gives the application role the minimum permissions it needs:

```bash
psql "postgres://lantern_migrate:changeme_migrate@localhost:5432/lantern?sslmode=disable" \
    -f migrations/grants.sql
```

This file grants `SELECT, INSERT, UPDATE, DELETE` on all tables and `USAGE, SELECT` on all sequences to `lantern_app`. It also sets `ALTER DEFAULT PRIVILEGES` so future tables created by `lantern_migrate` are automatically accessible.

**This step must be re-run whenever a new migration adds new tables.** The `setup-db.sh` script runs it every time, which is idempotent.

---

## SECURITY DEFINER Functions (Migration 004)

Migration 004 creates 4 functions that run as `lantern_migrate` (bypassing RLS) to handle cases where the application needs to look up a resource's organization before it can open an org-scoped transaction:

| Function | Purpose |
|----------|---------|
| `get_team_org_id(team_id, user_id)` | Resolves a team's org; verifies the user is a member |
| `get_project_org_id(project_id, user_id)` | Resolves a project's org; verifies the user is a member |
| `get_api_key_by_prefix(prefix)` | Returns an API key row for middleware auth; org is unknown at auth time |
| `touch_api_key_last_used(key_id)` | Updates `last_used_at`; best-effort in a goroutine |

**Why SECURITY DEFINER?**

All tenant-scoped tables have RLS enforced. The RLS policy is:
```sql
USING (organization_id = current_setting('lantern.current_organization_id', true)::uuid)
```

This means the application must `SET LOCAL lantern.current_organization_id = '<org_id>'` inside a transaction before querying those tables. But to set the org context, the app must first *know* the org — which it can't learn by querying the RLS-protected tables without the context already set.

SECURITY DEFINER functions break this chicken-and-egg problem by running as the table owner (who bypasses RLS) and returning only the data the caller is authorized to see (e.g., `get_team_org_id` inner-joins `organization_memberships` to ensure the requesting user is actually in the org).

---

## Environment Variables for the Application

After setup, export these before starting the collector or running integration tests:

```bash
export LANTERN_DATABASE_URL="postgres://lantern_app:changeme_app@localhost:5432/lantern?sslmode=disable"
export LANTERN_API_KEY_PEPPER="$(openssl rand -hex 32)"  # generate once; keep stable
```

For integration tests only:
```bash
LANTERN_DATABASE_URL="..." \
LANTERN_API_KEY_PEPPER="..." \
go test ./internal/integration/... -timeout 120s
```

The pepper must remain stable across restarts — changing it invalidates all existing API keys (because key hashes are computed as `SHA-256(key + pepper)`).

---

## Known Gotchas

**`COPY FROM` is incompatible with RLS.** PostgreSQL disallows `COPY FROM` into tables with RLS enabled when the role does not have `BYPASSRLS`. The coverage event insertion uses regular `INSERT ... ON CONFLICT DO NOTHING` instead of pgx's CopyFrom protocol for this reason.

**`SET LOCAL` does not accept `$1` parameters.** PostgreSQL's `SET` and `SET LOCAL` commands take literal values, not query parameters. The application uses `fmt.Sprintf` to interpolate the org UUID directly into the SQL string. Since UUIDs are hex digits and hyphens only, there is no SQL injection risk.

**Enum parameters in `CASE WHEN $n IN (...)`.** When a `$n` placeholder appears in both `SET col = $n` (binding it to an enum type) and `CASE WHEN $n IN ('a','b')` (binding it to text), PostgreSQL raises `42P08: inconsistent types deduced for parameter`. Use `$n = ANY(ARRAY[...]::enum_type[])` instead.

**`audit_log` uses RLS.** The audit log table is subject to the same tenant isolation policy as operational tables. Audit inserts must happen within an org-scoped transaction and use a context that carries the correct org ID. If the session's "primary org" differs from the resource being operated on, the audit insert will fail and roll back the transaction unless the context org is explicitly overridden before calling `audit.Log`.

---

## Teardown

To reset the database completely (destructive):

```bash
psql "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable" <<SQL
DROP DATABASE IF EXISTS lantern;
DROP ROLE IF EXISTS lantern_app;
DROP ROLE IF EXISTS lantern_migrate;
SQL
```

Then re-run `./scripts/setup-db.sh` to start fresh.
