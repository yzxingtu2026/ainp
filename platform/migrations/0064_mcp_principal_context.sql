-- 0064_mcp_principal_context.sql
-- Bind MCP OAuth grants and confirmations to the registered principal and source surface.

ALTER TABLE mcp_oauth_grants
  ADD COLUMN IF NOT EXISTS principal_kind varchar(128),
  ADD COLUMN IF NOT EXISTS client_surface varchar(128),
  ADD COLUMN IF NOT EXISTS audience varchar(255);

ALTER TABLE mcp_action_confirmations
  ADD COLUMN IF NOT EXISTS principal_kind varchar(128),
  ADD COLUMN IF NOT EXISTS client_surface varchar(128),
  ADD COLUMN IF NOT EXISTS audience varchar(255);

-- Existing encrypted confirmations used an older AAD contract. Revoke any
-- still-actionable records instead of attempting to reinterpret them.
UPDATE mcp_action_confirmations
SET status = 'revoked', arguments_ciphertext = ''
WHERE status IN ('pending', 'approved')
  AND (principal_kind IS NULL OR client_surface IS NULL OR audience IS NULL);

UPDATE mcp_oauth_grants
SET principal_kind = 'ainp.account', client_surface = 'ainp.tenant-console', audience = 'ainp:tenant-console'
WHERE principal_kind IS NULL OR client_surface IS NULL OR audience IS NULL;

UPDATE mcp_action_confirmations
SET principal_kind = 'ainp.account', client_surface = 'ainp.tenant-console', audience = 'ainp:tenant-console'
WHERE principal_kind IS NULL OR client_surface IS NULL OR audience IS NULL;

ALTER TABLE mcp_oauth_grants
  ALTER COLUMN principal_kind SET DEFAULT 'ainp.account',
  ALTER COLUMN principal_kind SET NOT NULL,
  ALTER COLUMN client_surface SET DEFAULT 'ainp.tenant-console',
  ALTER COLUMN client_surface SET NOT NULL,
  ALTER COLUMN audience SET DEFAULT 'ainp:tenant-console',
  ALTER COLUMN audience SET NOT NULL;

ALTER TABLE mcp_action_confirmations
  ALTER COLUMN principal_kind SET DEFAULT 'ainp.account',
  ALTER COLUMN principal_kind SET NOT NULL,
  ALTER COLUMN client_surface SET DEFAULT 'ainp.tenant-console',
  ALTER COLUMN client_surface SET NOT NULL,
  ALTER COLUMN audience SET DEFAULT 'ainp:tenant-console',
  ALTER COLUMN audience SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mcp_grant_principal
  ON mcp_oauth_grants (tenant_id, principal_kind, account_id, client_surface, status);
CREATE INDEX IF NOT EXISTS idx_mcp_confirmation_principal
  ON mcp_action_confirmations (tenant_id, principal_kind, account_id, client_surface, status);
