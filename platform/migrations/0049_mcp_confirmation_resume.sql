ALTER TABLE mcp_action_confirmations
  ADD COLUMN IF NOT EXISTS arguments_ciphertext text NOT NULL DEFAULT '';

ALTER TABLE mcp_action_confirmations
  ALTER COLUMN arguments_ciphertext DROP DEFAULT;
