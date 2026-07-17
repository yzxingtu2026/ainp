ALTER TABLE ai_model_invocations ADD COLUMN IF NOT EXISTS input_modalities_json jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE ai_model_invocations ADD COLUMN IF NOT EXISTS input_attachment_count integer NOT NULL DEFAULT 0;
ALTER TABLE ai_model_invocations ADD COLUMN IF NOT EXISTS input_attachment_bytes bigint NOT NULL DEFAULT 0;
