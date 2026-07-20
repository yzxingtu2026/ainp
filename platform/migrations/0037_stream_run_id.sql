-- 0037: propagate run_id from assistant stream to committed message
-- Fixes memory extraction: EvaluateRun queries assistant messages by run_id,
-- but stream-committed messages previously had run_id = NULL.

ALTER TABLE ai_assistant_streams ADD COLUMN IF NOT EXISTS run_id VARCHAR(64);
CREATE INDEX IF NOT EXISTS idx_ai_assistant_streams_run_id ON ai_assistant_streams (run_id);
