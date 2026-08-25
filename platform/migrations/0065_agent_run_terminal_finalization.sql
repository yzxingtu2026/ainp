ALTER TABLE ai_agent_runs
    ADD COLUMN IF NOT EXISTS terminal_protocol_version varchar(16) NOT NULL DEFAULT 'v1',
    ADD COLUMN IF NOT EXISTS terminal_reason varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS finalization_mode varchar(16) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS finalization_attempted boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS retryable boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS user_message_key varchar(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS safe_args jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS safe_summary varchar(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS terminal_message_id varchar(64),
    ADD COLUMN IF NOT EXISTS completed_operation_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS failed_operation_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS unfinished_operation_count integer NOT NULL DEFAULT 0;

ALTER TABLE ai_agent_runs DROP CONSTRAINT IF EXISTS ck_ai_agent_runs_finalization_mode;
ALTER TABLE ai_agent_runs
    ADD CONSTRAINT ck_ai_agent_runs_finalization_mode
    CHECK (finalization_mode IN ('model', 'template', 'notice_only', 'none'));

CREATE INDEX IF NOT EXISTS idx_ai_agent_runs_terminal_reason
    ON ai_agent_runs (tenant_id, terminal_reason, finished_at DESC);
