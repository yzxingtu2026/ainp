CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;

CREATE TABLE IF NOT EXISTS navigation_search_documents (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  menu_id varchar(128) NOT NULL,
  document_kind varchar(16) NOT NULL,
  locale varchar(16) NOT NULL,
  use_case varchar(160) NOT NULL DEFAULT '',
  title varchar(255) NOT NULL,
  content text NOT NULL,
  help_version varchar(32) NOT NULL DEFAULT '',
  content_hash varchar(64) NOT NULL,
  index_status varchar(16) NOT NULL,
  catalog_revision bigint NOT NULL DEFAULT 1,
  embedding_model varchar(128) NOT NULL DEFAULT '',
  embedding_alias varchar(64) NOT NULL DEFAULT '',
  embedding_deployment_id varchar(64) NOT NULL DEFAULT '',
  embedding_dimensions integer NOT NULL DEFAULT 0,
  embedding_preprocessing varchar(32) NOT NULL DEFAULT '',
  embedding_schema_version integer NOT NULL DEFAULT 0,
  embedding_vector public.vector,
  vector_updated_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CONSTRAINT ux_navigation_document UNIQUE (tenant_id, menu_id, document_kind, locale, use_case),
  CONSTRAINT ck_navigation_document_kind CHECK (document_kind IN ('page', 'help')),
  CONSTRAINT ck_navigation_index_status CHECK (index_status IN ('pending', 'ready', 'failed', 'stale'))
);

CREATE INDEX IF NOT EXISTS idx_navigation_search_vector_ready
  ON navigation_search_documents(tenant_id, locale, embedding_alias, embedding_deployment_id, embedding_dimensions)
  WHERE index_status = 'ready' AND embedding_vector IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_navigation_search_menu ON navigation_search_documents(tenant_id, menu_id);
