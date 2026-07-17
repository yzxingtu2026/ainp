CREATE TABLE IF NOT EXISTS ai_agent_runs (
 id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL,
 conversation_id varchar(64) NOT NULL, conversation_revision bigint NOT NULL, trigger_message_id varchar(64) NOT NULL, agent_version_id varchar(64) NOT NULL,
 session_id varchar(64) NOT NULL DEFAULT '', auth_version bigint NOT NULL, context_snapshot_id varchar(64),
 status varchar(32) NOT NULL, budget_json jsonb NOT NULL, current_step integer NOT NULL DEFAULT 0,
 model_calls integer NOT NULL DEFAULT 0, tool_calls integer NOT NULL DEFAULT 0, compactions integer NOT NULL DEFAULT 0,
 input_tokens bigint NOT NULL DEFAULT 0, output_tokens bigint NOT NULL DEFAULT 0,
 idempotency_key varchar(191) NOT NULL, lease_owner varchar(128) NOT NULL DEFAULT '', lease_expires_at timestamptz,
 error_category varchar(64) NOT NULL DEFAULT '', revision bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), started_at timestamptz, finished_at timestamptz,
 UNIQUE(tenant_id,user_id,idempotency_key),
 CONSTRAINT ck_ai_run_status CHECK(status IN ('queued','running','waiting_confirmation','paused','completed','failed','cancelled')),
 FOREIGN KEY(conversation_id) REFERENCES ai_conversations(id) ON DELETE RESTRICT,
 FOREIGN KEY(trigger_message_id) REFERENCES ai_messages(id) ON DELETE RESTRICT,
 FOREIGN KEY(agent_version_id) REFERENCES ai_agent_versions(id) ON DELETE RESTRICT,
 FOREIGN KEY(context_snapshot_id) REFERENCES ai_context_snapshots(id) ON DELETE RESTRICT
);
ALTER TABLE ai_tool_invocations ADD COLUMN IF NOT EXISTS result_json jsonb NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX IF NOT EXISTS idx_ai_agent_runs_list ON ai_agent_runs(tenant_id,user_id,status,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_agent_runs_lease ON ai_agent_runs(status,lease_expires_at);

CREATE TABLE IF NOT EXISTS ai_agent_steps (
 id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, run_id varchar(64) NOT NULL,
 step_index integer NOT NULL, type varchar(32) NOT NULL, status varchar(32) NOT NULL, parent_step_id varchar(64),
 model_invocation_id varchar(64), tool_invocation_id varchar(64), confirmation_id varchar(64), context_snapshot_id varchar(64),
 input_summary jsonb NOT NULL DEFAULT '{}'::jsonb, output_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
 error_category varchar(64) NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), started_at timestamptz, finished_at timestamptz,
 UNIQUE(tenant_id,run_id,step_index),
 CONSTRAINT ck_ai_step_type CHECK(type IN ('context_assemble','context_compact','model','tool_search','tool_load','tool_execute','confirmation','finalize')),
 CONSTRAINT ck_ai_step_status CHECK(status IN ('pending','running','waiting','completed','failed','skipped','cancelled')),
 FOREIGN KEY(run_id) REFERENCES ai_agent_runs(id) ON DELETE RESTRICT,
 FOREIGN KEY(parent_step_id) REFERENCES ai_agent_steps(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_agent_steps_run ON ai_agent_steps(tenant_id,run_id,step_index);

CREATE TABLE IF NOT EXISTS ai_agent_run_events (
 id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, run_id varchar(64) NOT NULL,
 sequence bigint NOT NULL, type varchar(64) NOT NULL, step_id varchar(64), payload jsonb NOT NULL DEFAULT '{}'::jsonb,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(tenant_id,run_id,sequence),
 FOREIGN KEY(run_id) REFERENCES ai_agent_runs(id) ON DELETE RESTRICT,
 FOREIGN KEY(step_id) REFERENCES ai_agent_steps(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_agent_run_events_cursor ON ai_agent_run_events(tenant_id,run_id,sequence);
