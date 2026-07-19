CREATE TABLE IF NOT EXISTS ai_memory_items (
 id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL, agent_id varchar(64),
 type varchar(24) NOT NULL, status varchar(24) NOT NULL, summary varchar(512) NOT NULL, content text NOT NULL,
 confidence double precision NOT NULL, sensitivity varchar(16) NOT NULL, importance integer NOT NULL, revision bigint NOT NULL DEFAULT 1,
 occurred_at timestamptz, occurred_end_at timestamptz, occurred_timezone varchar(64) NOT NULL DEFAULT '', local_date date,
 date_precision varchar(16) NOT NULL, observed_at timestamptz NOT NULL, recorded_at timestamptz NOT NULL,
 valid_from timestamptz, valid_to timestamptz, expires_at timestamptz, last_recalled_at timestamptz, recall_count bigint NOT NULL DEFAULT 0,
 supersedes_item_id varchar(64), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT ck_ai_memory_type CHECK (type IN ('preference','fact','context','event')),
 CONSTRAINT ck_ai_memory_status CHECK (status IN ('candidate','pending_review','active','superseded','revoked','expired','deleted')),
 CONSTRAINT ck_ai_memory_confidence CHECK (confidence >= 0 AND confidence <= 1), CONSTRAINT ck_ai_memory_importance CHECK (importance BETWEEN 0 AND 100),
 CONSTRAINT ck_ai_memory_date_precision CHECK (date_precision IN ('exact_time','exact_day','range','approximate','unknown')),
 CONSTRAINT ck_ai_memory_occurred_range CHECK (occurred_end_at IS NULL OR occurred_at IS NULL OR occurred_end_at >= occurred_at),
 CONSTRAINT ck_ai_memory_validity CHECK (valid_to IS NULL OR valid_from IS NULL OR valid_to > valid_from),
 UNIQUE (tenant_id, user_id, id), UNIQUE (tenant_id, id),
 FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id, agent_id) REFERENCES ai_agents (tenant_id, id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id, user_id, supersedes_item_id) REFERENCES ai_memory_items (tenant_id, user_id, id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_memory_current ON ai_memory_items (tenant_id,user_id,status,type,importance DESC,recorded_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_model_invocations_tenant_id ON ai_model_invocations (tenant_id,id);

CREATE TABLE IF NOT EXISTS ai_memory_item_versions (
 id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL, item_id varchar(64) NOT NULL,
 revision bigint NOT NULL, summary varchar(512) NOT NULL, content text NOT NULL, content_hash varchar(64) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (tenant_id,item_id,revision), FOREIGN KEY (tenant_id,user_id,item_id) REFERENCES ai_memory_items (tenant_id,user_id,id) ON DELETE RESTRICT
);
CREATE TABLE IF NOT EXISTS ai_memory_evidence (
 id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL, item_id varchar(64) NOT NULL,
 source_kind varchar(24) NOT NULL, conversation_id varchar(64), message_id varchar(64), run_id varchar(64), agent_id varchar(64), agent_version_id varchar(64), actor_user_id varchar(64), extraction_invocation_id varchar(64),
 citation varchar(512) NOT NULL DEFAULT '', content_hash varchar(64) NOT NULL, source_revision bigint NOT NULL DEFAULT 1, revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT ck_ai_memory_source CHECK (source_kind IN ('user_statement','assistant_inference','tool','knowledge','web')),
 FOREIGN KEY (tenant_id,user_id,item_id) REFERENCES ai_memory_items (tenant_id,user_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,conversation_id) REFERENCES ai_conversations (tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,message_id) REFERENCES ai_messages (tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,run_id) REFERENCES ai_agent_runs (tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,agent_id) REFERENCES ai_agents (tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,agent_version_id) REFERENCES ai_agent_versions (tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,actor_user_id) REFERENCES users (tenant_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (tenant_id,extraction_invocation_id) REFERENCES ai_model_invocations (tenant_id,id) ON DELETE RESTRICT
);
CREATE TABLE IF NOT EXISTS ai_memory_projections (
 id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL, item_id varchar(64) NOT NULL,
 kind varchar(24) NOT NULL, status varchar(16) NOT NULL, item_revision bigint NOT NULL, content_hash varchar(64) NOT NULL,
 model_alias varchar(64) NOT NULL DEFAULT '', deployment_id varchar(64) NOT NULL DEFAULT '', dimensions integer NOT NULL DEFAULT 0,
 search_text text, vector_json jsonb, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (tenant_id,item_id,kind,item_revision), FOREIGN KEY (tenant_id,user_id,item_id) REFERENCES ai_memory_items (tenant_id,user_id,id) ON DELETE RESTRICT,
 CONSTRAINT ck_ai_memory_projection_kind CHECK (kind IN ('exact','fts','vector','day_summary','current_profile')),
 CONSTRAINT ck_ai_memory_projection_status CHECK (status IN ('pending','ready','stale','deleted')),
 CONSTRAINT ck_ai_memory_vector_meta CHECK (kind <> 'vector' OR (model_alias <> '' AND deployment_id <> '' AND dimensions > 0))
);
CREATE TABLE IF NOT EXISTS ai_memory_day_summaries (id varchar(64) PRIMARY KEY,tenant_id varchar(64) NOT NULL,user_id varchar(64) NOT NULL,agent_id varchar(64),local_date date NOT NULL,timezone varchar(64) NOT NULL,source_revision bigint NOT NULL,status varchar(16) NOT NULL,summary text NOT NULL,content_hash varchar(64) NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(tenant_id,user_id,agent_id,local_date,source_revision),FOREIGN KEY(tenant_id,user_id) REFERENCES users(tenant_id,id) ON DELETE RESTRICT,FOREIGN KEY(tenant_id,agent_id) REFERENCES ai_agents(tenant_id,id) ON DELETE RESTRICT,CHECK(status IN ('pending','ready','stale','deleted')));
CREATE TABLE IF NOT EXISTS ai_memory_current_profiles (id varchar(64) PRIMARY KEY,tenant_id varchar(64) NOT NULL,user_id varchar(64) NOT NULL,agent_id varchar(64),source_revision bigint NOT NULL,status varchar(16) NOT NULL,profile_json jsonb NOT NULL,content_hash varchar(64) NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(tenant_id,user_id,agent_id,source_revision),FOREIGN KEY(tenant_id,user_id) REFERENCES users(tenant_id,id) ON DELETE RESTRICT,FOREIGN KEY(tenant_id,agent_id) REFERENCES ai_agents(tenant_id,id) ON DELETE RESTRICT,CHECK(status IN ('pending','ready','stale','deleted')));
