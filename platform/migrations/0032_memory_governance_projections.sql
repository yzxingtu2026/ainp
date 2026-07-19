ALTER TABLE ai_memory_items DROP CONSTRAINT IF EXISTS ck_ai_memory_status;
ALTER TABLE ai_memory_items ADD CONSTRAINT ck_ai_memory_status CHECK (status IN ('candidate','pending_review','active','superseded','revoked','expired','deleted','rejected'));
ALTER TABLE ai_memory_items ADD COLUMN IF NOT EXISTS attribute_key varchar(64) NOT NULL DEFAULT '';
ALTER TABLE ai_memory_items ADD COLUMN IF NOT EXISTS attribute_value varchar(512) NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_memory_items_tenant_user_id ON ai_memory_items(tenant_id,user_id,id);

CREATE TABLE IF NOT EXISTS ai_memory_governance_decisions(
 id varchar(64) PRIMARY KEY,tenant_id varchar(64) NOT NULL,user_id varchar(64) NOT NULL,item_id varchar(64) NOT NULL,
 item_revision bigint NOT NULL,action varchar(32) NOT NULL,reason varchar(64) NOT NULL,related_item_id varchar(64),
 policy_version varchar(32) NOT NULL,idempotency_key varchar(191) NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,idempotency_key),
 CHECK(action IN ('activate','pending_review','reject','duplicate','supersede','revoke','expire')),
 FOREIGN KEY(tenant_id,user_id,item_id) REFERENCES ai_memory_items(tenant_id,user_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(tenant_id,user_id,related_item_id) REFERENCES ai_memory_items(tenant_id,user_id,id) ON DELETE RESTRICT
);
CREATE TABLE IF NOT EXISTS ai_memory_relations(
 id varchar(64) PRIMARY KEY,tenant_id varchar(64) NOT NULL,user_id varchar(64) NOT NULL,
 from_item_id varchar(64) NOT NULL,to_item_id varchar(64) NOT NULL,kind varchar(24) NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,from_item_id,to_item_id,kind),CHECK(kind IN ('duplicate','conflict','supersedes')),
 FOREIGN KEY(tenant_id,user_id,from_item_id) REFERENCES ai_memory_items(tenant_id,user_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(tenant_id,user_id,to_item_id) REFERENCES ai_memory_items(tenant_id,user_id,id) ON DELETE RESTRICT
);
CREATE TABLE IF NOT EXISTS ai_memory_consolidation_batches(
 id varchar(64) PRIMARY KEY,tenant_id varchar(64) NOT NULL,user_id varchar(64) NOT NULL,status varchar(24) NOT NULL,
 watermark_at timestamptz,lease_owner varchar(128) NOT NULL DEFAULT '',lease_expires_at timestamptz,invocation_id varchar(64),
 plan_json jsonb NOT NULL DEFAULT '{}',attempts integer NOT NULL DEFAULT 0,error_category varchar(64) NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(tenant_id,user_id),
 CHECK(status IN ('pending','running','completed','failed')),CHECK(attempts>=0),
 FOREIGN KEY(tenant_id,user_id) REFERENCES users(tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(tenant_id,invocation_id) REFERENCES ai_model_invocations(tenant_id,id) ON DELETE RESTRICT
);

ALTER TABLE ai_memory_day_summaries ADD COLUMN IF NOT EXISTS date_group varchar(32) NOT NULL DEFAULT 'exact';
ALTER TABLE ai_memory_day_summaries ADD COLUMN IF NOT EXISTS policy_version varchar(32) NOT NULL DEFAULT '';
ALTER TABLE ai_memory_day_summaries ADD COLUMN IF NOT EXISTS model_invocation_id varchar(64);
ALTER TABLE ai_memory_day_summaries ADD COLUMN IF NOT EXISTS source_items_json jsonb NOT NULL DEFAULT '[]';
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_memory_day_policy ON ai_memory_day_summaries(tenant_id,user_id,local_date,timezone,policy_version,date_group);
ALTER TABLE ai_memory_current_profiles ADD COLUMN IF NOT EXISTS policy_version varchar(32) NOT NULL DEFAULT '';
ALTER TABLE ai_memory_current_profiles ADD COLUMN IF NOT EXISTS model_invocation_id varchar(64);
ALTER TABLE ai_memory_current_profiles ADD COLUMN IF NOT EXISTS source_items_json jsonb NOT NULL DEFAULT '[]';
CREATE INDEX IF NOT EXISTS idx_ai_memory_governance_pending ON ai_memory_items(tenant_id,user_id,status,recorded_at) WHERE status IN ('candidate','pending_review');
