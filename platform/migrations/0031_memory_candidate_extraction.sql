ALTER TABLE ai_memory_items ADD COLUMN IF NOT EXISTS evaluation_id varchar(64);
ALTER TABLE ai_memory_items ADD COLUMN IF NOT EXISTS agent_version_id varchar(64);
ALTER TABLE ai_memory_items ADD COLUMN IF NOT EXISTS policy_json jsonb;
ALTER TABLE ai_memory_items ADD COLUMN IF NOT EXISTS prompt_version varchar(32) NOT NULL DEFAULT '';
ALTER TABLE ai_memory_items ADD COLUMN IF NOT EXISTS schema_version varchar(32) NOT NULL DEFAULT '';
ALTER TABLE ai_memory_items ADD COLUMN IF NOT EXISTS arguments_hash varchar(64) NOT NULL DEFAULT '';
ALTER TABLE ai_memory_items ADD COLUMN IF NOT EXISTS explicit_user_intent boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS ai_memory_evaluations (
 id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL,
 conversation_id varchar(64) NOT NULL, run_id varchar(64) NOT NULL, agent_version_id varchar(64) NOT NULL,
 sequence_from bigint NOT NULL, sequence_to bigint NOT NULL, status varchar(24) NOT NULL,
 candidate_count integer NOT NULL DEFAULT 0, invocation_id varchar(64), arguments_hash varchar(64) NOT NULL DEFAULT '',
 error_category varchar(64) NOT NULL DEFAULT '', attempts integer NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 UNIQUE (tenant_id,run_id), UNIQUE (tenant_id,id),
 CHECK (status IN ('running','completed','failed')),
 CHECK (sequence_from <= sequence_to), CHECK (candidate_count >= 0), CHECK (attempts >= 0),
 FOREIGN KEY (tenant_id,user_id) REFERENCES users(tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,conversation_id) REFERENCES ai_conversations(tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,run_id) REFERENCES ai_agent_runs(tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,agent_version_id) REFERENCES ai_agent_versions(tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,invocation_id) REFERENCES ai_model_invocations(tenant_id,id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_memory_evaluation_backlog ON ai_memory_evaluations(tenant_id,status,updated_at);

ALTER TABLE ai_memory_items DROP CONSTRAINT IF EXISTS fk_ai_memory_items_evaluation_tenant;
ALTER TABLE ai_memory_items DROP CONSTRAINT IF EXISTS fk_ai_memory_items_version_tenant;
ALTER TABLE ai_memory_items ADD CONSTRAINT fk_ai_memory_items_evaluation_tenant FOREIGN KEY(tenant_id,evaluation_id) REFERENCES ai_memory_evaluations(tenant_id,id) ON DELETE RESTRICT;
ALTER TABLE ai_memory_items ADD CONSTRAINT fk_ai_memory_items_version_tenant FOREIGN KEY(tenant_id,agent_version_id) REFERENCES ai_agent_versions(tenant_id,id) ON DELETE RESTRICT;
