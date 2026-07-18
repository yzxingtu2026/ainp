-- Issue #99: deterministic exact/FTS recall over current knowledge payloads.
ALTER TABLE ai_knowledge_chunk_payloads
  ADD COLUMN IF NOT EXISTS search_vector tsvector
  GENERATED ALWAYS AS (to_tsvector('simple', text)) STORED;
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_chunk_payload_fts
  ON ai_knowledge_chunk_payloads USING gin (search_vector);
