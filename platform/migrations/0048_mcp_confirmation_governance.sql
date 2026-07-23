CREATE TABLE IF NOT EXISTS mcp_action_confirmations (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  user_id varchar(64) NOT NULL,
  grant_id varchar(64) NOT NULL REFERENCES mcp_oauth_grants(id),
  client_id varchar(64) NOT NULL,
  action_id varchar(128) NOT NULL,
  tool_name varchar(128) NOT NULL,
  schema_hash varchar(64) NOT NULL,
  arguments_hash varchar(64) NOT NULL,
  recovery_hash varchar(64) NOT NULL UNIQUE,
  title varchar(160) NOT NULL,
  description varchar(512) NOT NULL,
  risk varchar(16) NOT NULL,
  side_effect varchar(16) NOT NULL,
  status varchar(16) NOT NULL,
  expires_at timestamptz NOT NULL,
  decided_at timestamptz,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CONSTRAINT ck_mcp_confirmation_status CHECK (status IN ('pending','approved','rejected','consumed','revoked','expired'))
);
CREATE INDEX IF NOT EXISTS idx_mcp_confirmation_owner ON mcp_action_confirmations(tenant_id,user_id,status,expires_at);
CREATE INDEX IF NOT EXISTS idx_mcp_confirmation_grant ON mcp_action_confirmations(grant_id,status);
