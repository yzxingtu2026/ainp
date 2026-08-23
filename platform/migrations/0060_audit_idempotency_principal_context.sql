ALTER TABLE audit_logs
  ADD COLUMN IF NOT EXISTS principal_kind varchar(128),
  ADD COLUMN IF NOT EXISTS client_surface varchar(128),
  ADD COLUMN IF NOT EXISTS audience varchar(255);

UPDATE audit_logs
SET principal_kind = 'ainp.account'
WHERE principal_kind IS NULL AND actor_id IS NOT NULL AND btrim(actor_id) <> '';

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_principal
  ON audit_logs (tenant_id, principal_kind, actor_id, occurred_at DESC);

ALTER TABLE idempotency_records
  ADD COLUMN IF NOT EXISTS principal_kind varchar(128),
  ADD COLUMN IF NOT EXISTS client_surface varchar(128),
  ADD COLUMN IF NOT EXISTS audience varchar(255);

UPDATE idempotency_records
SET principal_kind = 'ainp.account'
WHERE principal_kind IS NULL;

ALTER TABLE idempotency_records
  ALTER COLUMN principal_kind SET NOT NULL;

DROP INDEX IF EXISTS ux_idempotency_records_key;
DROP INDEX IF EXISTS ux_idempotency_key;
CREATE UNIQUE INDEX ux_idempotency_key
  ON idempotency_records (tenant_id, principal_kind, principal_id, action_id, idempotency_key);
