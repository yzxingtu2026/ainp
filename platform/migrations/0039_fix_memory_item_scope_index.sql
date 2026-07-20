DROP INDEX IF EXISTS ux_ai_memory_item_scope;

CREATE INDEX IF NOT EXISTS idx_ai_memory_scope
  ON ai_memory_items (tenant_id, user_id, agent_id);
