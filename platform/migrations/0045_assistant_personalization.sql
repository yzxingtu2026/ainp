CREATE TABLE IF NOT EXISTS ai_assistant_personalization_snapshots (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL, slot_key varchar(128) NOT NULL,
  agent_version_id varchar(64) NOT NULL DEFAULT '', locale varchar(16) NOT NULL, timezone varchar(64) NOT NULL,
  context_scope varchar(32) NOT NULL DEFAULT 'global', context_hash varchar(64) NOT NULL DEFAULT '', greeting varchar(240) NOT NULL,
  suggestions_json jsonb NOT NULL DEFAULT '[]', source_versions_json jsonb NOT NULL DEFAULT '{}', model_alias varchar(64) NOT NULL,
  prompt_version varchar(32) NOT NULL, status varchar(16) NOT NULL, generation_revision bigint NOT NULL DEFAULT 1,
  generated_at timestamptz NOT NULL, expires_at timestamptz NOT NULL, last_error_category varchar(64) NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  CONSTRAINT ux_ai_assistant_personalization UNIQUE (tenant_id, user_id, slot_key),
  CONSTRAINT ck_ai_assistant_personalization_status CHECK (status IN ('ready','stale','failed'))
);
CREATE INDEX IF NOT EXISTS idx_ai_assistant_personalization_expiry ON ai_assistant_personalization_snapshots(tenant_id, expires_at);
