ALTER TABLE ai_model_invocations
  ADD COLUMN IF NOT EXISTS structured_output_strategy varchar(32) NOT NULL DEFAULT '';
