ALTER TABLE auth_sessions
  ADD COLUMN IF NOT EXISTS revoked_reason varchar(64) NOT NULL DEFAULT '';

ALTER TABLE auth_sessions
  DROP CONSTRAINT IF EXISTS ck_auth_sessions_revoked_reason;

ALTER TABLE auth_sessions
  ADD CONSTRAINT ck_auth_sessions_revoked_reason CHECK (
    revoked_reason IN ('', 'idle_expired', 'absolute_expired', 'overflow')
  );

CREATE INDEX IF NOT EXISTS idx_auth_sessions_active_surface
  ON auth_sessions (tenant_id, principal_kind, principal_id, client_surface, created_at, id)
  WHERE revoked_at IS NULL;
