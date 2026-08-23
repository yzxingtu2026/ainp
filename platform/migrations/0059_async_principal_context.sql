ALTER TABLE outbox_events
  ADD COLUMN IF NOT EXISTS principal_kind varchar(128),
  ADD COLUMN IF NOT EXISTS client_surface varchar(128),
  ADD COLUMN IF NOT EXISTS audience varchar(255);

UPDATE outbox_events
SET principal_kind = 'ainp.account'
WHERE principal_kind IS NULL AND actor_id IS NOT NULL AND btrim(actor_id) <> '';

ALTER TABLE job_runs
  ADD COLUMN IF NOT EXISTS principal_kind varchar(128),
  ADD COLUMN IF NOT EXISTS client_surface varchar(128),
  ADD COLUMN IF NOT EXISTS audience varchar(255);

UPDATE job_runs
SET principal_kind = 'ainp.account'
WHERE principal_kind IS NULL AND created_by IS NOT NULL AND btrim(created_by) <> '';

CREATE INDEX IF NOT EXISTS idx_outbox_principal ON outbox_events (tenant_id, principal_kind, actor_id);
CREATE INDEX IF NOT EXISTS idx_job_runs_principal ON job_runs (tenant_id, principal_kind, created_by);
