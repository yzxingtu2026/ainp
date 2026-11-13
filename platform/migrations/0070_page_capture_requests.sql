-- Add bounded client-fulfilled page capture requests for Agent Runs (Issue #403).

CREATE TABLE IF NOT EXISTS ai_page_capture_requests (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  principal_id varchar(64) NOT NULL,
  principal_kind varchar(128) NOT NULL DEFAULT 'ainp.account',
  client_surface varchar(128) NOT NULL DEFAULT '',
  audience varchar(255) NOT NULL DEFAULT '',
  session_id varchar(64) NOT NULL DEFAULT '',
  auth_version bigint NOT NULL,
  run_id varchar(64) NOT NULL,
  step_id varchar(64) NOT NULL,
  scope_id varchar(64) NOT NULL,
  invocation_id varchar(64) NOT NULL,
  tool_call_id varchar(128) NOT NULL,
  tool_id varchar(255) NOT NULL,
  parameters_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  status varchar(24) NOT NULL,
  attachment_id varchar(64),
  attachment_revision varchar(128) NOT NULL DEFAULT '',
  error_category varchar(64) NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL,
  submitted_at timestamptz,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ck_ai_page_capture_requests_status CHECK (status IN ('pending', 'submitted', 'consumed', 'failed', 'expired', 'cancelled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_page_capture_requests_tenant_id
  ON ai_page_capture_requests (tenant_id, id);

CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_page_capture_requests_invocation
  ON ai_page_capture_requests (tenant_id, invocation_id);

CREATE INDEX IF NOT EXISTS idx_ai_page_capture_requests_run_status
  ON ai_page_capture_requests (tenant_id, run_id, status, created_at DESC);

ALTER TABLE ai_page_capture_requests
  DROP CONSTRAINT IF EXISTS fk_ai_page_capture_requests_run_tenant,
  DROP CONSTRAINT IF EXISTS fk_ai_page_capture_requests_step_tenant,
  DROP CONSTRAINT IF EXISTS fk_ai_page_capture_requests_scope_tenant,
  DROP CONSTRAINT IF EXISTS fk_ai_page_capture_requests_invocation_tenant,
  ADD CONSTRAINT fk_ai_page_capture_requests_run_tenant FOREIGN KEY (tenant_id, run_id) REFERENCES ai_agent_runs (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_page_capture_requests_step_tenant FOREIGN KEY (tenant_id, step_id) REFERENCES ai_agent_steps (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_page_capture_requests_scope_tenant FOREIGN KEY (tenant_id, scope_id) REFERENCES ai_tool_scopes (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_page_capture_requests_invocation_tenant FOREIGN KEY (tenant_id, invocation_id) REFERENCES ai_tool_invocations (tenant_id, id) ON DELETE RESTRICT;
