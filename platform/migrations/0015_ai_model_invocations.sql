CREATE TABLE IF NOT EXISTS ai_model_invocations (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL REFERENCES tenants(id),
  user_id varchar(64) NOT NULL,
  session_id varchar(64) NOT NULL DEFAULT '',
  agent_id varchar(64), agent_run_id varchar(64),
  model_alias varchar(64) NOT NULL,
  provider_id varchar(64) NOT NULL,
  deployment_id varchar(64) NOT NULL,
  remote_model_id varchar(255) NOT NULL,
  mode varchar(16) NOT NULL CHECK (mode IN ('generate','stream')),
  status varchar(16) NOT NULL CHECK (status IN ('running','completed','failed','cancelled','unknown')),
  started_at timestamptz NOT NULL,
  first_token_at timestamptz,
  finished_at timestamptz,
  latency_ms bigint,
  first_token_latency_ms bigint,
  input_tokens bigint, output_tokens bigint, total_tokens bigint,
  cached_input_tokens bigint, reasoning_tokens bigint,
  usage_reported boolean NOT NULL DEFAULT false,
  finish_reason varchar(32) NOT NULL DEFAULT '',
  error_category varchar(64) NOT NULL DEFAULT '',
  provider_request_id varchar(255) NOT NULL DEFAULT '',
  request_id varchar(64) NOT NULL DEFAULT '',
  trace_id varchar(64) NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT fk_ai_invocation_provider_tenant FOREIGN KEY (tenant_id, provider_id) REFERENCES ai_providers(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_invocation_deployment_tenant FOREIGN KEY (tenant_id, deployment_id) REFERENCES ai_model_deployments(tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_invocations_tenant_started ON ai_model_invocations(tenant_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_invocations_tenant_status ON ai_model_invocations(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_ai_invocations_request ON ai_model_invocations(request_id);
CREATE INDEX IF NOT EXISTS idx_ai_invocations_trace ON ai_model_invocations(trace_id);
