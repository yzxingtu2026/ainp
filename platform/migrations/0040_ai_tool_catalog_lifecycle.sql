ALTER TABLE ai_tool_catalog
  ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 1;

ALTER TABLE ai_tool_search_documents
  ADD COLUMN IF NOT EXISTS catalog_revision bigint NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_ai_tool_search_status
  ON ai_tool_search_documents(tenant_id, index_status, catalog_revision);
