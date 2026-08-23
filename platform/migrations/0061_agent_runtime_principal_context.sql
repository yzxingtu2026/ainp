-- Preserve the authenticated principal and client surface when Agent Runs are resumed by workers.
ALTER TABLE ai_agent_runs ADD COLUMN IF NOT EXISTS principal_kind varchar(128) NOT NULL DEFAULT 'ainp.account';
ALTER TABLE ai_agent_runs ADD COLUMN IF NOT EXISTS client_surface varchar(128) NOT NULL DEFAULT '';
ALTER TABLE ai_agent_runs ADD COLUMN IF NOT EXISTS audience varchar(255) NOT NULL DEFAULT '';
DROP INDEX IF EXISTS ux_ai_run_idem;
CREATE UNIQUE INDEX ux_ai_run_idem ON ai_agent_runs (tenant_id, user_id, idempotency_key, principal_kind);
ALTER TABLE ai_tool_scopes ADD COLUMN IF NOT EXISTS principal_kind varchar(128) NOT NULL DEFAULT 'ainp.account';
ALTER TABLE ai_tool_scopes ADD COLUMN IF NOT EXISTS client_surface varchar(128) NOT NULL DEFAULT '';
ALTER TABLE ai_tool_scopes ADD COLUMN IF NOT EXISTS audience varchar(255) NOT NULL DEFAULT '';
