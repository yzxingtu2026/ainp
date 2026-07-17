-- Agent Builder 实时预览：版本 kind + 会话 purpose 隔离。
-- preview 版本不晋升 current_version；builder_preview 会话与 Workspace 隔离。

ALTER TABLE ai_agent_versions
  ADD COLUMN IF NOT EXISTS kind varchar(16) NOT NULL DEFAULT 'release',
  ADD COLUMN IF NOT EXISTS content_hash varchar(64) NOT NULL DEFAULT '';

UPDATE ai_agent_versions SET kind = 'release' WHERE kind = '' OR kind IS NULL;

ALTER TABLE ai_agent_versions DROP CONSTRAINT IF EXISTS ck_ai_agent_versions_kind;
ALTER TABLE ai_agent_versions
  ADD CONSTRAINT ck_ai_agent_versions_kind CHECK (kind IN ('release', 'preview'));

CREATE INDEX IF NOT EXISTS idx_ai_agent_versions_preview_hash
  ON ai_agent_versions (tenant_id, agent_id, kind, content_hash);

ALTER TABLE ai_conversations
  ADD COLUMN IF NOT EXISTS purpose varchar(32) NOT NULL DEFAULT 'workspace';

UPDATE ai_conversations SET purpose = 'workspace' WHERE purpose = '' OR purpose IS NULL;

ALTER TABLE ai_conversations DROP CONSTRAINT IF EXISTS ck_ai_conversations_purpose;
ALTER TABLE ai_conversations
  ADD CONSTRAINT ck_ai_conversations_purpose CHECK (purpose IN ('workspace', 'builder_preview'));

CREATE INDEX IF NOT EXISTS idx_ai_conversations_purpose
  ON ai_conversations (tenant_id, purpose, status);
