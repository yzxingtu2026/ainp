ALTER TABLE ai_agent_runs
    ADD COLUMN IF NOT EXISTS execution_mode varchar(32) NOT NULL DEFAULT 'normal',
    ADD COLUMN IF NOT EXISTS completion_reason varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS completion_attempted boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS completion_reserved_input_tokens bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS completion_reserved_output_tokens bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS completion_safety_margin_tokens bigint NOT NULL DEFAULT 0;

ALTER TABLE ai_agent_runs
    DROP CONSTRAINT IF EXISTS ck_ai_agent_runs_execution_mode;
ALTER TABLE ai_agent_runs
    ADD CONSTRAINT ck_ai_agent_runs_execution_mode
    CHECK (execution_mode IN ('normal', 'completion_only'));

ALTER TABLE ai_agent_runs
    DROP CONSTRAINT IF EXISTS ck_ai_agent_runs_completion_reserve;
ALTER TABLE ai_agent_runs
    ADD CONSTRAINT ck_ai_agent_runs_completion_reserve
    CHECK (
        completion_reserved_input_tokens >= 0
        AND completion_reserved_output_tokens >= 0
        AND completion_safety_margin_tokens >= 0
    );
