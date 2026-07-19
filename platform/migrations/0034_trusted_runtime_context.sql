ALTER TABLE ai_agent_runs
  ADD COLUMN IF NOT EXISTS evaluated_at_utc timestamptz,
  ADD COLUMN IF NOT EXISTS timezone varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS time_snapshot_json jsonb NOT NULL DEFAULT '{}';

ALTER TABLE ai_context_snapshots
  ADD COLUMN IF NOT EXISTS runtime_context_refs jsonb NOT NULL DEFAULT '{}';
