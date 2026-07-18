-- Issue #96: AI knowledge foundation metadata only (no source text, vector or FTS payload).
CREATE UNIQUE INDEX IF NOT EXISTS ux_users_tenant_id ON users (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_attachments_tenant_id ON attachments (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_agents_tenant_id ON ai_agents (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_model_deployments_tenant_id ON ai_model_deployments (tenant_id, id);

CREATE TABLE IF NOT EXISTS ai_knowledge_bases (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  owner_user_id varchar(64) NOT NULL,
  code varchar(64) NOT NULL,
  name varchar(128) NOT NULL,
  description varchar(1024) NOT NULL DEFAULT '',
  visibility varchar(16) NOT NULL,
  status varchar(16) NOT NULL DEFAULT 'draft',
  embedding_alias varchar(64) NOT NULL DEFAULT '',
  chunking_config jsonb NOT NULL,
  retrieval_config jsonb NOT NULL,
  current_index_version_id varchar(64),
  revision bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  CONSTRAINT ux_ai_knowledge_bases_tenant_id UNIQUE (tenant_id, id),
  CONSTRAINT fk_ai_knowledge_bases_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_knowledge_base_owner_tenant FOREIGN KEY (tenant_id, owner_user_id) REFERENCES users(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT ck_ai_knowledge_base_visibility CHECK (visibility IN ('private', 'tenant')),
  CONSTRAINT ck_ai_knowledge_base_status CHECK (status IN ('draft', 'enabled', 'disabled')),
  CONSTRAINT ck_ai_knowledge_base_enabled_embedding CHECK (status <> 'enabled' OR embedding_alias <> ''),
  CONSTRAINT ck_ai_knowledge_base_revision CHECK (revision > 0),
  CONSTRAINT ck_ai_knowledge_base_chunking_config CHECK (
    jsonb_typeof(chunking_config) = 'object'
    AND chunking_config ?& ARRAY['strategy', 'chunk_size', 'chunk_overlap']
  ),
  CONSTRAINT ck_ai_knowledge_base_retrieval_config CHECK (
    jsonb_typeof(retrieval_config) = 'object'
    AND retrieval_config ?& ARRAY['top_k', 'min_score', 'max_context_tokens']
  )
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_knowledge_bases_tenant_code ON ai_knowledge_bases (tenant_id, code) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_bases_tenant_visibility ON ai_knowledge_bases (tenant_id, visibility);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_bases_tenant_owner ON ai_knowledge_bases (tenant_id, owner_user_id);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_bases_tenant_status ON ai_knowledge_bases (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_bases_deleted_at ON ai_knowledge_bases (deleted_at);

CREATE TABLE IF NOT EXISTS ai_knowledge_documents (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  knowledge_base_id varchar(64) NOT NULL,
  attachment_id varchar(64) NOT NULL,
  source_revision varchar(64) NOT NULL,
  content_type varchar(128) NOT NULL,
  original_name varchar(512) NOT NULL,
  ingestion_status varchar(24) NOT NULL DEFAULT 'pending',
  ingestion_error_category varchar(64) NOT NULL DEFAULT '',
  current_index_version_id varchar(64),
  revision bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  CONSTRAINT ux_ai_knowledge_documents_tenant_id UNIQUE (tenant_id, id),
  CONSTRAINT ux_ai_knowledge_documents_base_id UNIQUE (tenant_id, knowledge_base_id, id),
  CONSTRAINT fk_ai_knowledge_documents_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_knowledge_document_base_tenant FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES ai_knowledge_bases(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_knowledge_document_attachment_tenant FOREIGN KEY (tenant_id, attachment_id) REFERENCES attachments(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT ck_ai_knowledge_document_source_revision CHECK (source_revision ~ '^[a-f0-9]{64}$'),
  CONSTRAINT ck_ai_knowledge_document_status CHECK (ingestion_status IN ('pending', 'processing', 'ready', 'failed')),
  CONSTRAINT ck_ai_knowledge_document_error_category CHECK (ingestion_error_category = '' OR ingestion_error_category ~ '^[a-z][a-z0-9_.-]{0,63}$'),
  CONSTRAINT ck_ai_knowledge_document_revision CHECK (revision > 0)
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_knowledge_documents_attachment ON ai_knowledge_documents (tenant_id, knowledge_base_id, attachment_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_documents_base_status ON ai_knowledge_documents (tenant_id, knowledge_base_id, ingestion_status);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_documents_attachment ON ai_knowledge_documents (tenant_id, attachment_id);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_documents_deleted_at ON ai_knowledge_documents (deleted_at);

CREATE TABLE IF NOT EXISTS ai_knowledge_index_versions (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  knowledge_base_id varchar(64) NOT NULL,
  version integer NOT NULL,
  status varchar(24) NOT NULL DEFAULT 'pending',
  embedding_alias varchar(64) NOT NULL,
  embedding_deployment_id varchar(64) NOT NULL,
  embedding_dimensions integer NOT NULL,
  preprocessing_version varchar(32) NOT NULL,
  parser_version varchar(32) NOT NULL,
  chunking_version varchar(32) NOT NULL,
  document_count integer NOT NULL DEFAULT 0,
  chunk_count integer NOT NULL DEFAULT 0,
  config_hash varchar(64) NOT NULL,
  error_category varchar(64) NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  CONSTRAINT ux_ai_knowledge_index_versions_tenant_id UNIQUE (tenant_id, id),
  CONSTRAINT ux_ai_knowledge_index_versions_base_id UNIQUE (tenant_id, knowledge_base_id, id),
  CONSTRAINT ux_ai_knowledge_index_versions_base_version UNIQUE (tenant_id, knowledge_base_id, version),
  CONSTRAINT fk_ai_knowledge_index_versions_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_knowledge_index_base_tenant FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES ai_knowledge_bases(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_knowledge_index_embedding_deployment_tenant FOREIGN KEY (tenant_id, embedding_deployment_id) REFERENCES ai_model_deployments(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT ck_ai_knowledge_index_version CHECK (version > 0),
  CONSTRAINT ck_ai_knowledge_index_status CHECK (status IN ('pending', 'building', 'ready', 'failed', 'superseded', 'aborted')),
  CONSTRAINT ck_ai_knowledge_index_dimensions CHECK (embedding_dimensions > 0),
  CONSTRAINT ck_ai_knowledge_index_counts CHECK (document_count >= 0 AND chunk_count >= 0),
  CONSTRAINT ck_ai_knowledge_index_versions_metadata CHECK (
    embedding_deployment_id <> '' AND preprocessing_version <> '' AND parser_version <> '' AND chunking_version <> ''
  ),
  CONSTRAINT ck_ai_knowledge_index_config_hash CHECK (config_hash ~ '^[a-f0-9]{64}$'),
  CONSTRAINT ck_ai_knowledge_index_error_category CHECK (error_category = '' OR error_category ~ '^[a-z][a-z0-9_.-]{0,63}$')
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_index_versions_base_status ON ai_knowledge_index_versions (tenant_id, knowledge_base_id, status);

DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'fk_ai_knowledge_base_current_index_tenant'
      AND conrelid = 'ai_knowledge_bases'::regclass
  ) THEN
    ALTER TABLE ai_knowledge_bases
      ADD CONSTRAINT fk_ai_knowledge_base_current_index_tenant
      FOREIGN KEY (tenant_id, id, current_index_version_id)
      REFERENCES ai_knowledge_index_versions(tenant_id, knowledge_base_id, id) ON DELETE RESTRICT;
  END IF;
END $$;
DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'fk_ai_knowledge_document_current_index_tenant'
      AND conrelid = 'ai_knowledge_documents'::regclass
  ) THEN
    ALTER TABLE ai_knowledge_documents
      ADD CONSTRAINT fk_ai_knowledge_document_current_index_tenant
      FOREIGN KEY (tenant_id, knowledge_base_id, current_index_version_id)
      REFERENCES ai_knowledge_index_versions(tenant_id, knowledge_base_id, id) ON DELETE RESTRICT;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS ai_knowledge_chunks (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  knowledge_base_id varchar(64) NOT NULL,
  document_id varchar(64) NOT NULL,
  index_version_id varchar(64) NOT NULL,
  ordinal integer NOT NULL,
  source_revision varchar(64) NOT NULL,
  page_number integer,
  position_start integer,
  position_end integer,
  heading_path varchar(1024) NOT NULL DEFAULT '',
  token_count integer NOT NULL DEFAULT 0,
  content_hash varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ux_ai_knowledge_chunks_position UNIQUE (tenant_id, index_version_id, document_id, ordinal),
  CONSTRAINT fk_ai_knowledge_chunks_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_knowledge_chunk_base_tenant FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES ai_knowledge_bases(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_knowledge_chunk_document_tenant FOREIGN KEY (tenant_id, knowledge_base_id, document_id) REFERENCES ai_knowledge_documents(tenant_id, knowledge_base_id, id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_knowledge_chunk_index_tenant FOREIGN KEY (tenant_id, knowledge_base_id, index_version_id) REFERENCES ai_knowledge_index_versions(tenant_id, knowledge_base_id, id) ON DELETE RESTRICT,
  CONSTRAINT ck_ai_knowledge_chunk_ordinal CHECK (ordinal >= 0),
  CONSTRAINT ck_ai_knowledge_chunk_source_revision CHECK (source_revision ~ '^[a-f0-9]{64}$'),
  CONSTRAINT ck_ai_knowledge_chunk_page CHECK (page_number IS NULL OR page_number > 0),
  CONSTRAINT ck_ai_knowledge_chunk_positions CHECK (
    (position_start IS NULL AND position_end IS NULL)
    OR (position_start >= 0 AND position_end > position_start)
  ),
  CONSTRAINT ck_ai_knowledge_chunk_token_count CHECK (token_count >= 0),
  CONSTRAINT ck_ai_knowledge_chunk_content_hash CHECK (content_hash ~ '^[a-f0-9]{64}$')
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_chunks_document ON ai_knowledge_chunks (tenant_id, document_id);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_chunks_index ON ai_knowledge_chunks (tenant_id, index_version_id);

CREATE TABLE IF NOT EXISTS ai_agent_knowledge_bindings (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  agent_id varchar(64) NOT NULL,
  knowledge_base_id varchar(64) NOT NULL,
  created_by varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ux_ai_agent_knowledge_binding UNIQUE (tenant_id, agent_id, knowledge_base_id),
  CONSTRAINT fk_ai_agent_knowledge_bindings_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_agent_knowledge_agent_tenant FOREIGN KEY (tenant_id, agent_id) REFERENCES ai_agents(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_agent_knowledge_base_tenant FOREIGN KEY (tenant_id, knowledge_base_id) REFERENCES ai_knowledge_bases(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_agent_knowledge_creator_tenant FOREIGN KEY (tenant_id, created_by) REFERENCES users(tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_agent_knowledge_bindings_base ON ai_agent_knowledge_bindings (tenant_id, knowledge_base_id);
