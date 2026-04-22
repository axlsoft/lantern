-- Migration 001: Organizations, users, teams, memberships, sessions, and auth tokens.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "citext";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ── Organizations ─────────────────────────────────────────────────────────────

CREATE TABLE organizations (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        TEXT        NOT NULL,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Users ─────────────────────────────────────────────────────────────────────

CREATE TABLE users (
    id                  UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    email               CITEXT      NOT NULL UNIQUE,
    password_hash       TEXT        NOT NULL,
    email_verified_at   TIMESTAMPTZ,
    totp_secret         TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Teams ─────────────────────────────────────────────────────────────────────

CREATE TABLE teams (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT        NOT NULL,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_teams_organization_id ON teams(organization_id);

-- ── Memberships ───────────────────────────────────────────────────────────────

CREATE TYPE org_role AS ENUM ('owner', 'admin', 'member', 'viewer');
CREATE TYPE team_role AS ENUM ('admin', 'member');

CREATE TABLE organization_memberships (
    user_id         UUID     NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id UUID     NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role            org_role NOT NULL DEFAULT 'member',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, organization_id)
);

CREATE INDEX idx_org_memberships_org_id ON organization_memberships(organization_id);

CREATE TABLE team_memberships (
    user_id     UUID      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team_id     UUID      NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    role        team_role NOT NULL DEFAULT 'member',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, team_id)
);

CREATE INDEX idx_team_memberships_team_id ON team_memberships(team_id);

-- ── Sessions ──────────────────────────────────────────────────────────────────

CREATE TABLE sessions (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_agent   TEXT,
    ip_address   INET
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- ── Email verifications ───────────────────────────────────────────────────────

CREATE TABLE email_verifications (
    token       UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ
);

CREATE INDEX idx_email_verifications_user_id ON email_verifications(user_id);

-- ── Password resets ───────────────────────────────────────────────────────────

CREATE TABLE password_resets (
    token       UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ
);

CREATE INDEX idx_password_resets_user_id ON password_resets(user_id);

-- ── Org invites ───────────────────────────────────────────────────────────────

CREATE TABLE org_invites (
    token           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    invited_email   CITEXT      NOT NULL,
    role            org_role    NOT NULL DEFAULT 'member',
    invited_by      UUID        NOT NULL REFERENCES users(id),
    expires_at      TIMESTAMPTZ NOT NULL,
    accepted_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_org_invites_org_id ON org_invites(organization_id);
