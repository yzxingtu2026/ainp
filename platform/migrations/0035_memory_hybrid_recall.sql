ALTER TABLE ai_context_snapshots ADD COLUMN IF NOT EXISTS memory_recall_refs jsonb NOT NULL DEFAULT '[]';
CREATE INDEX IF NOT EXISTS idx_ai_memory_recall_hard_filter ON ai_memory_items (tenant_id,user_id,status,local_date,type) WHERE status='active';
CREATE INDEX IF NOT EXISTS idx_ai_memory_projection_recall ON ai_memory_projections (tenant_id,user_id,kind,status,item_id);
