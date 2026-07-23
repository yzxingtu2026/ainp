CREATE TABLE IF NOT EXISTS mcp_oauth_clients (
  id varchar(64) PRIMARY KEY,
  name varchar(160) NOT NULL,
  redirect_uris jsonb NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS mcp_oauth_grants (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  user_id varchar(64) NOT NULL,
  client_id varchar(64) NOT NULL,
  scopes varchar(128) NOT NULL,
  auth_version bigint NOT NULL,
  status varchar(16) NOT NULL,
  expires_at timestamptz NOT NULL,
  last_used_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CONSTRAINT ck_mcp_grant_status CHECK (status IN ('active','revoked','expired')),
  CONSTRAINT fk_mcp_grant_client FOREIGN KEY (client_id) REFERENCES mcp_oauth_clients(id)
);
CREATE INDEX IF NOT EXISTS idx_mcp_grant_owner ON mcp_oauth_grants(tenant_id,user_id,status);

CREATE TABLE IF NOT EXISTS mcp_oauth_authorization_codes (
  id varchar(64) PRIMARY KEY,
  code_hash varchar(64) NOT NULL UNIQUE,
  grant_id varchar(64) NOT NULL REFERENCES mcp_oauth_grants(id),
  client_id varchar(64) NOT NULL,
  redirect_uri varchar(1024) NOT NULL,
  code_challenge varchar(128) NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS mcp_oauth_token_families (
  id varchar(64) PRIMARY KEY,
  grant_id varchar(64) NOT NULL REFERENCES mcp_oauth_grants(id),
  refresh_token_hash varchar(64) NOT NULL UNIQUE,
  previous_refresh_token_hash varchar(64),
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
