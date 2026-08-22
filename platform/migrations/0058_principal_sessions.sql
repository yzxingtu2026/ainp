-- Generalize framework sessions without reusing account_id as a business-principal fact.
-- Existing Account sessions are backfilled in place and remain valid.

ALTER TABLE auth_sessions
  ADD COLUMN IF NOT EXISTS principal_kind varchar(128),
  ADD COLUMN IF NOT EXISTS principal_id varchar(255),
  ADD COLUMN IF NOT EXISTS client_surface varchar(128),
  ADD COLUMN IF NOT EXISTS audience varchar(255),
  ADD COLUMN IF NOT EXISTS token_family_id varchar(64);

UPDATE auth_sessions
SET principal_kind = 'ainp.account',
    principal_id = account_id,
    client_surface = 'ainp.tenant-console',
    audience = 'ainp:tenant-console',
    token_family_id = id
WHERE principal_kind IS NULL
   OR principal_id IS NULL
   OR client_surface IS NULL
   OR audience IS NULL
   OR token_family_id IS NULL;

ALTER TABLE auth_sessions
  ALTER COLUMN principal_kind SET NOT NULL,
  ALTER COLUMN principal_id SET NOT NULL,
  ALTER COLUMN client_surface SET NOT NULL,
  ALTER COLUMN audience SET NOT NULL,
  ALTER COLUMN token_family_id SET NOT NULL,
  ALTER COLUMN account_id DROP NOT NULL;

ALTER TABLE auth_sessions
  DROP CONSTRAINT IF EXISTS ck_auth_sessions_principal_kind,
  DROP CONSTRAINT IF EXISTS ck_auth_sessions_principal_id,
  DROP CONSTRAINT IF EXISTS ck_auth_sessions_client_surface,
  DROP CONSTRAINT IF EXISTS ck_auth_sessions_audience,
  ADD CONSTRAINT ck_auth_sessions_principal_kind CHECK (principal_kind ~ '^[a-z][a-z0-9]*([.-][a-z][a-z0-9]*)+$'),
  ADD CONSTRAINT ck_auth_sessions_principal_id CHECK (length(btrim(principal_id)) > 0),
  ADD CONSTRAINT ck_auth_sessions_client_surface CHECK (length(btrim(client_surface)) > 0),
  ADD CONSTRAINT ck_auth_sessions_audience CHECK (length(btrim(audience)) > 0);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_principal
  ON auth_sessions (tenant_id, principal_kind, principal_id);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_token_family
  ON auth_sessions (token_family_id);

CREATE TABLE IF NOT EXISTS auth_refresh_token_uses (
  token_hash varchar(64) PRIMARY KEY,
  session_id varchar(64) NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
  token_family_id varchar(64) NOT NULL,
  used_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_auth_refresh_token_uses_session ON auth_refresh_token_uses (session_id);
CREATE INDEX IF NOT EXISTS idx_auth_refresh_token_uses_family ON auth_refresh_token_uses (token_family_id);

ALTER TABLE auth_sessions
  DROP CONSTRAINT IF EXISTS ck_auth_sessions_account_principal,
  ADD CONSTRAINT ck_auth_sessions_account_principal CHECK (
    (principal_kind = 'ainp.account' AND account_id = principal_id)
    OR (principal_kind <> 'ainp.account' AND account_id IS NULL)
  );
