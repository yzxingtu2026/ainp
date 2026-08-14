ALTER TABLE job_runs
    ADD COLUMN IF NOT EXISTS error_category VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS error_stage VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE job_runs DROP CONSTRAINT IF EXISTS ck_job_runs_error_category;
ALTER TABLE job_runs ADD CONSTRAINT ck_job_runs_error_category
    CHECK (error_category = '' OR error_category ~ '^[a-z][a-z0-9_.-]{0,63}$');

ALTER TABLE job_runs DROP CONSTRAINT IF EXISTS ck_job_runs_error_stage;
ALTER TABLE job_runs ADD CONSTRAINT ck_job_runs_error_stage
    CHECK (error_stage = '' OR error_stage ~ '^[a-z][a-z0-9_.-]{0,63}$');

ALTER TABLE ai_tool_search_documents
    ADD COLUMN IF NOT EXISTS error_category VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS error_stage VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE navigation_search_documents
    ADD COLUMN IF NOT EXISTS error_category VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS error_stage VARCHAR(64) NOT NULL DEFAULT '';
