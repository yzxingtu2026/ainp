-- Issue #98: derived knowledge payloads and idempotent source revision builds.
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
ALTER TABLE ai_knowledge_index_versions ADD COLUMN IF NOT EXISTS source_document_id varchar(64);
ALTER TABLE ai_knowledge_index_versions ADD COLUMN IF NOT EXISTS source_revision varchar(64);

CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_knowledge_index_source_revision
  ON ai_knowledge_index_versions (tenant_id, knowledge_base_id, source_document_id, source_revision);
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'ck_ai_knowledge_index_source_revision') THEN
    ALTER TABLE ai_knowledge_index_versions ADD CONSTRAINT ck_ai_knowledge_index_source_revision CHECK (source_revision IS NULL OR source_revision ~ '^[a-f0-9]{64}$');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_ai_knowledge_index_source_document_tenant') THEN
    ALTER TABLE ai_knowledge_index_versions ADD CONSTRAINT fk_ai_knowledge_index_source_document_tenant
      FOREIGN KEY (tenant_id, knowledge_base_id, source_document_id)
      REFERENCES ai_knowledge_documents(tenant_id, knowledge_base_id, id) ON DELETE RESTRICT;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS ai_knowledge_chunk_payloads (
  chunk_id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  index_version_id varchar(64) NOT NULL,
  text text NOT NULL,
  vector public.vector NOT NULL,
  dimensions integer NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ck_ai_knowledge_chunk_payload_dimensions CHECK (dimensions > 0),
  CONSTRAINT fk_ai_knowledge_chunk_payload_chunk FOREIGN KEY (chunk_id) REFERENCES ai_knowledge_chunks(id) ON DELETE CASCADE,
  CONSTRAINT fk_ai_knowledge_chunk_payload_index FOREIGN KEY (tenant_id, index_version_id) REFERENCES ai_knowledge_index_versions(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_chunk_payload_index ON ai_knowledge_chunk_payloads (tenant_id, index_version_id);
