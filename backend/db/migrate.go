package db

import "database/sql"

func Migrate(database *sql.DB) error {
	_, err := database.Exec(schema)
	return err
}

const schema = `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    username VARCHAR(100),
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS username VARCHAR(100);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_unique ON users (username) WHERE username IS NOT NULL;

CREATE TABLE IF NOT EXISTS api_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Logins rotate the user's token with the same name, so enforce one per
-- (user, name). First remove any pre-existing duplicates, keeping the newest.
DELETE FROM api_tokens a
USING api_tokens b
WHERE a.user_id = b.user_id
  AND a.name = b.name
  AND (a.created_at < b.created_at
       OR (a.created_at = b.created_at AND a.id < b.id));

CREATE UNIQUE INDEX IF NOT EXISTS api_tokens_user_name_key ON api_tokens (user_id, name);

CREATE TABLE IF NOT EXISTS webauthn_users (
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    rp_id VARCHAR(512) NOT NULL,
    user_handle BYTEA NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, rp_id),
    UNIQUE (rp_id, user_handle)
);

CREATE TABLE IF NOT EXISTS passkeys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    rp_id VARCHAR(512) NOT NULL,
    name VARCHAR(100) NOT NULL,
    credential_id BYTEA NOT NULL,
    credential JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP WITH TIME ZONE,
    UNIQUE (rp_id, credential_id)
);

CREATE TABLE IF NOT EXISTS webauthn_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    kind VARCHAR(32) NOT NULL,
    label VARCHAR(100),
    session_json JSONB NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, slug)
);

CREATE TABLE IF NOT EXISTS project_transfers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    sender_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP WITH TIME ZONE,
    CHECK (status IN ('pending', 'accepted', 'declined', 'canceled', 'expired')),
    CHECK (sender_user_id <> recipient_user_id)
);

CREATE TABLE IF NOT EXISTS project_members (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invited_by UUID REFERENCES users(id) ON DELETE SET NULL,
    joined_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX IF NOT EXISTS project_members_user_idx ON project_members (user_id, joined_at DESC);

CREATE TABLE IF NOT EXISTS project_share_invitations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    sender_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP WITH TIME ZONE,
    CHECK (status IN ('pending', 'accepted', 'declined', 'canceled', 'expired')),
    CHECK (sender_user_id <> recipient_user_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS project_share_one_pending_idx
    ON project_share_invitations (project_id, recipient_user_id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS project_share_recipient_idx
    ON project_share_invitations (recipient_user_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS project_share_sender_idx
    ON project_share_invitations (sender_user_id, status, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS project_transfers_one_pending_idx
    ON project_transfers (project_id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS project_transfers_recipient_idx
    ON project_transfers (recipient_user_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS project_transfers_sender_idx
    ON project_transfers (sender_user_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS env_versions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    environment_name VARCHAR(50) NOT NULL,
    version INT NOT NULL,
    encrypted_payload TEXT NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, environment_name, version)
);

CREATE TABLE IF NOT EXISTS activity_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    action VARCHAR(50) NOT NULL,
    project_slug VARCHAR(100),
    environment_name VARCHAR(50),
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS activity_logs_user_created_idx ON activity_logs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS activity_logs_project_idx ON activity_logs (project_id);
`
