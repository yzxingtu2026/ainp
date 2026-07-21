ALTER TABLE ai_agent_runs
    ADD COLUMN IF NOT EXISTS page_context_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS page_context_type varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS page_context_status varchar(16) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS page_context_bytes integer NOT NULL DEFAULT 0;

ALTER TABLE ai_context_snapshots
    ADD COLUMN IF NOT EXISTS page_context_refs jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_ai_agent_runs_page_context_type
    ON ai_agent_runs (tenant_id, page_context_type)
    WHERE page_context_type <> '';

ALTER TABLE ai_agent_runs
    DROP CONSTRAINT IF EXISTS ck_ai_agent_runs_page_context_status;
ALTER TABLE ai_agent_runs
    ADD CONSTRAINT ck_ai_agent_runs_page_context_status
    CHECK (page_context_status IN ('none', 'resolved'));

ALTER TABLE ai_agent_runs
    DROP CONSTRAINT IF EXISTS ck_ai_agent_runs_page_context_bytes;
ALTER TABLE ai_agent_runs
    ADD CONSTRAINT ck_ai_agent_runs_page_context_bytes
    CHECK (page_context_bytes >= 0 AND page_context_bytes <= 8192);
