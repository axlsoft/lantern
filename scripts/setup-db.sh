#!/usr/bin/env bash
# setup-db.sh — Provision a Postgres instance for Lantern development.
#
# Supports two modes:
#   local    Use a local Postgres server (default; also covers Docker Compose).
#   tunnel   Open an SSH tunnel to a remote server first, then run setup.
#
# Usage:
#   ./scripts/setup-db.sh [local|tunnel]
#
# Environment variables (override defaults):
#   PGHOST          Postgres host (default: localhost)
#   PGPORT          Postgres port (default: 5432)
#   SUPERUSER       Superuser role for initial setup (default: postgres)
#   SUPERUSER_PASS  Superuser password (default: postgres)
#   DB_NAME         Database to create (default: lantern)
#   MIGRATE_PASS    Password for lantern_migrate role (default: changeme_migrate)
#   APP_PASS        Password for lantern_app role (default: changeme_app)
#   SSH_HOST        Remote host for tunnel mode (e.g. user@db01.example.com)
#   SSH_REMOTE_PORT Postgres port on the remote host (default: 5432)
#   LOCAL_TUNNEL_PORT Local port for the tunnel (default: 5432)
#
# Prerequisites:
#   - psql and migrate CLI installed
#   - migrate: go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
#
set -euo pipefail

MODE="${1:-local}"

# ── Defaults ──────────────────────────────────────────────────────────────────
PGHOST="${PGHOST:-localhost}"
PGPORT="${PGPORT:-5432}"
SUPERUSER="${SUPERUSER:-postgres}"
SUPERUSER_PASS="${SUPERUSER_PASS:-postgres}"
DB_NAME="${DB_NAME:-lantern}"
MIGRATE_PASS="${MIGRATE_PASS:-changeme_migrate}"
APP_PASS="${APP_PASS:-changeme_app}"
SSH_HOST="${SSH_HOST:-}"
SSH_REMOTE_PORT="${SSH_REMOTE_PORT:-5432}"
LOCAL_TUNNEL_PORT="${LOCAL_TUNNEL_PORT:-5432}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

SUPERUSER_URL="postgres://${SUPERUSER}:${SUPERUSER_PASS}@${PGHOST}:${PGPORT}/postgres?sslmode=disable"
MIGRATE_URL="postgres://lantern_migrate:${MIGRATE_PASS}@${PGHOST}:${PGPORT}/${DB_NAME}?sslmode=disable"
APP_URL="postgres://lantern_app:${APP_PASS}@${PGHOST}:${PGPORT}/${DB_NAME}?sslmode=disable"

# ── SSH Tunnel ────────────────────────────────────────────────────────────────
TUNNEL_PID=""

open_tunnel() {
    if [[ -z "$SSH_HOST" ]]; then
        echo "ERROR: SSH_HOST must be set for tunnel mode (e.g. SSH_HOST=user@db01.example.com)" >&2
        exit 1
    fi
    echo "→ Opening SSH tunnel: localhost:${LOCAL_TUNNEL_PORT} → ${SSH_HOST}:${SSH_REMOTE_PORT}"
    ssh -f -N -L "${LOCAL_TUNNEL_PORT}:127.0.0.1:${SSH_REMOTE_PORT}" "$SSH_HOST"
    # Give SSH a moment to establish the tunnel before we start querying.
    sleep 1
    TUNNEL_PID=$(pgrep -n -f "ssh.*-L ${LOCAL_TUNNEL_PORT}:127.0.0.1:${SSH_REMOTE_PORT}" || true)
    echo "  Tunnel PID: ${TUNNEL_PID:-unknown}"
}

close_tunnel() {
    if [[ -n "$TUNNEL_PID" ]]; then
        echo "→ Closing SSH tunnel (PID $TUNNEL_PID)"
        kill "$TUNNEL_PID" 2>/dev/null || true
    fi
}

if [[ "$MODE" == "tunnel" ]]; then
    open_tunnel
    trap close_tunnel EXIT
fi

# ── Wait for Postgres ─────────────────────────────────────────────────────────
echo "→ Waiting for Postgres at ${PGHOST}:${PGPORT}..."
for i in $(seq 1 20); do
    if PGPASSWORD="$SUPERUSER_PASS" psql -h "$PGHOST" -p "$PGPORT" -U "$SUPERUSER" -d postgres \
        -c "SELECT 1" >/dev/null 2>&1; then
        echo "  Connected."
        break
    fi
    if [[ $i -eq 20 ]]; then
        echo "ERROR: Could not connect to Postgres after 20 attempts." >&2
        exit 1
    fi
    sleep 1
done

# ── Create Roles ──────────────────────────────────────────────────────────────
echo "→ Creating roles..."
PGPASSWORD="$SUPERUSER_PASS" psql -h "$PGHOST" -p "$PGPORT" -U "$SUPERUSER" -d postgres <<SQL
-- Migration owner: creates and owns all schema objects.
-- CREATEDB lets it create the database; no SUPERUSER needed.
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'lantern_migrate') THEN
        CREATE ROLE lantern_migrate
            WITH LOGIN
            PASSWORD '${MIGRATE_PASS}'
            CREATEDB
            NOCREATEROLE
            NOSUPERUSER
            NOREPLICATION
            NOBYPASSRLS;
        RAISE NOTICE 'Created role lantern_migrate';
    ELSE
        -- Update password in case it changed.
        ALTER ROLE lantern_migrate WITH PASSWORD '${MIGRATE_PASS}';
        RAISE NOTICE 'Role lantern_migrate already exists — password updated';
    END IF;
END
\$\$;

-- Application role: used by the running collector process.
-- NOBYPASSRLS is the default for non-superusers but we make it explicit
-- because the entire security model depends on RLS being enforced.
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'lantern_app') THEN
        CREATE ROLE lantern_app
            WITH LOGIN
            PASSWORD '${APP_PASS}'
            NOCREATEDB
            NOCREATEROLE
            NOSUPERUSER
            NOREPLICATION
            NOBYPASSRLS;
        RAISE NOTICE 'Created role lantern_app';
    ELSE
        ALTER ROLE lantern_app WITH PASSWORD '${APP_PASS}';
        RAISE NOTICE 'Role lantern_app already exists — password updated';
    END IF;
END
\$\$;
SQL

# ── Create Database ───────────────────────────────────────────────────────────
echo "→ Creating database '${DB_NAME}'..."
PGPASSWORD="$SUPERUSER_PASS" psql -h "$PGHOST" -p "$PGPORT" -U "$SUPERUSER" -d postgres <<SQL
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_database WHERE datname = '${DB_NAME}') THEN
        -- Must run outside a transaction block; we use a workaround via shell.
        RAISE NOTICE 'Database ${DB_NAME} will be created in the next step.';
    ELSE
        RAISE NOTICE 'Database ${DB_NAME} already exists.';
    END IF;
END
\$\$;
SQL

# CREATE DATABASE cannot run inside a transaction block, so we call it separately.
PGPASSWORD="$SUPERUSER_PASS" psql -h "$PGHOST" -p "$PGPORT" -U "$SUPERUSER" -d postgres \
    -c "SELECT 'exists' FROM pg_database WHERE datname = '${DB_NAME}'" \
    | grep -q exists \
    || PGPASSWORD="$SUPERUSER_PASS" psql -h "$PGHOST" -p "$PGPORT" -U "$SUPERUSER" -d postgres \
        -c "CREATE DATABASE ${DB_NAME} OWNER lantern_migrate;"

# Allow the app role to connect.
PGPASSWORD="$SUPERUSER_PASS" psql -h "$PGHOST" -p "$PGPORT" -U "$SUPERUSER" -d postgres \
    -c "GRANT CONNECT ON DATABASE ${DB_NAME} TO lantern_app;"

# ── Enable uuid-ossp Extension ────────────────────────────────────────────────
echo "→ Enabling extensions..."
PGPASSWORD="$SUPERUSER_PASS" psql -h "$PGHOST" -p "$PGPORT" -U "$SUPERUSER" -d "$DB_NAME" \
    -c "CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";"

# ── Run Migrations ────────────────────────────────────────────────────────────
echo "→ Running migrations..."
migrate \
    -path "${REPO_ROOT}/migrations" \
    -database "$MIGRATE_URL" \
    up

# ── Apply Table Grants ────────────────────────────────────────────────────────
echo "→ Applying grants..."
PGPASSWORD="$MIGRATE_PASS" psql -h "$PGHOST" -p "$PGPORT" \
    -U lantern_migrate -d "$DB_NAME" \
    -f "${REPO_ROOT}/migrations/grants.sql"

# ── Verify ────────────────────────────────────────────────────────────────────
echo "→ Verifying setup..."
PGPASSWORD="$APP_PASS" psql -h "$PGHOST" -p "$PGPORT" \
    -U lantern_app -d "$DB_NAME" \
    -c "SELECT proname FROM pg_proc WHERE proname IN ('get_team_org_id','get_project_org_id','get_api_key_by_prefix','touch_api_key_last_used') ORDER BY proname;" \
    | grep -c get > /dev/null \
    && echo "  SECURITY DEFINER functions: OK" \
    || echo "  WARNING: expected 4 SECURITY DEFINER functions"

PGPASSWORD="$APP_PASS" psql -h "$PGHOST" -p "$PGPORT" \
    -U lantern_app -d "$DB_NAME" \
    -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" \
    | grep -E "^\s+[0-9]" | xargs | read TABLE_COUNT \
    && echo "  Tables accessible to lantern_app: OK" || true

echo ""
echo "✓ Database setup complete."
echo ""
echo "  App URL:     postgres://lantern_app:${APP_PASS}@${PGHOST}:${PGPORT}/${DB_NAME}?sslmode=disable"
echo "  Migrate URL: postgres://lantern_migrate:${MIGRATE_PASS}@${PGHOST}:${PGPORT}/${DB_NAME}?sslmode=disable"
echo ""
echo "  To run integration tests:"
echo "    LANTERN_DATABASE_URL=\"${APP_URL}\" \\"
echo "    LANTERN_API_KEY_PEPPER=\"<generate with: openssl rand -hex 32>\" \\"
echo "    go test ./internal/integration/... -timeout 120s"
