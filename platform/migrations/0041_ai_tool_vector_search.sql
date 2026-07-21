CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;

ALTER TABLE ai_tool_search_documents
  ADD COLUMN IF NOT EXISTS embedding_alias varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS embedding_deployment_id varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS embedding_dimensions integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS embedding_preprocessing varchar(32) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS embedding_schema_version integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS embedding_vector public.vector,
  ADD COLUMN IF NOT EXISTS vector_updated_at timestamptz;

CREATE INDEX IF NOT EXISTS idx_ai_tool_search_vector_ready
  ON ai_tool_search_documents(tenant_id, embedding_alias, embedding_deployment_id, embedding_dimensions)
  WHERE index_status = 'ready' AND embedding_vector IS NOT NULL;
