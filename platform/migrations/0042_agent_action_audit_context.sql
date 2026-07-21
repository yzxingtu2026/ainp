ALTER TABLE ai_agent_runs
    ADD COLUMN IF NOT EXISTS origin_request_id varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trace_id varchar(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_ai_agent_runs_origin_request_id
    ON ai_agent_runs (tenant_id, origin_request_id)
    WHERE origin_request_id <> '';

CREATE INDEX IF NOT EXISTS idx_ai_agent_runs_trace_id
    ON ai_agent_runs (tenant_id, trace_id)
    WHERE trace_id <> '';

ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS trace_id varchar(64),
    ADD COLUMN IF NOT EXISTS agent_run_id varchar(64),
    ADD COLUMN IF NOT EXISTS tool_invocation_id varchar(64);

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_trace_id
    ON audit_logs (tenant_id, trace_id)
    WHERE trace_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_agent_run_id
    ON audit_logs (tenant_id, agent_run_id)
    WHERE agent_run_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_tool_invocation_id
    ON audit_logs (tenant_id, tool_invocation_id)
    WHERE tool_invocation_id IS NOT NULL;
