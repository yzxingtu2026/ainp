-- 0012_authentication_sessions.sql
-- Unified authentication identities, challenges and rotating sessions.
-- Access tokens issued before this migration do not contain sid and require login again.

CREATE TABLE IF NOT EXISTS auth_identities (
    id varchar(64) PRIMARY KEY,
    tenant_id varchar(64) NOT NULL,
    user_id varchar(64) NOT NULL,
    provider varchar(64) NOT NULL,
    subject varchar(255) NOT NULL,
    normalized_identifier varchar(255) NOT NULL DEFAULT '',
    status varchar(32) NOT NULL DEFAULT 'enabled',
    verified_at timestamptz NOT NULL,
    metadata jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ux_auth_identity_subject UNIQUE (tenant_id, provider, subject),
    CONSTRAINT ck_auth_identities_status CHECK (status IN ('enabled','disabled'))
);
CREATE INDEX IF NOT EXISTS idx_auth_identities_user ON auth_identities (tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_auth_identities_identifier ON auth_identities (normalized_identifier);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id varchar(64) PRIMARY KEY,
    tenant_id varchar(64) NOT NULL,
    user_id varchar(64) NOT NULL,
    realm varchar(32) NOT NULL,
    client varchar(64) NOT NULL,
    device_summary varchar(255) NOT NULL DEFAULT '',
    refresh_token_hash varchar(64) NOT NULL UNIQUE,
    previous_refresh_token_hash varchar(64) NOT NULL DEFAULT '',
    auth_version bigint NOT NULL,
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_auth_sessions_realm CHECK (realm IN ('tenant','platform')),
    CONSTRAINT ck_auth_sessions_auth_version CHECK (auth_version > 0)
);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_owner ON auth_sessions (tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires ON auth_sessions (expires_at);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_previous_refresh ON auth_sessions (previous_refresh_token_hash);

CREATE TABLE IF NOT EXISTS auth_challenges (
    id varchar(64) PRIMARY KEY,
    method varchar(64) NOT NULL,
    secret_hash varchar(64) NOT NULL DEFAULT '',
    client varchar(64) NOT NULL DEFAULT '',
    attempts integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 5,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_auth_challenges_attempts CHECK (attempts >= 0 AND max_attempts > 0 AND attempts <= max_attempts)
);
CREATE INDEX IF NOT EXISTS idx_auth_challenges_method ON auth_challenges (method);
CREATE INDEX IF NOT EXISTS idx_auth_challenges_expires ON auth_challenges (expires_at);
