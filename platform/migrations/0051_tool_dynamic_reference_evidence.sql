ALTER TABLE ai_tool_scopes
    ADD COLUMN IF NOT EXISTS reference_evidence_json jsonb NOT NULL DEFAULT '[]'::jsonb;
