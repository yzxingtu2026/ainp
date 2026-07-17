CREATE TABLE IF NOT EXISTS ai_conversations (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, owner_user_id varchar(64) NOT NULL,
  title varchar(256) NOT NULL, agent_id varchar(64) NOT NULL, agent_version_id varchar(64) NOT NULL,
  visibility varchar(16) NOT NULL DEFAULT 'private', status varchar(16) NOT NULL DEFAULT 'active',
  last_message_at timestamptz, revision bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ck_ai_conversations_visibility CHECK (visibility IN ('private','tenant')),
  CONSTRAINT ck_ai_conversations_status CHECK (status IN ('active','archived')),
  FOREIGN KEY (agent_id) REFERENCES ai_agents(id) ON DELETE RESTRICT,
  FOREIGN KEY (agent_version_id) REFERENCES ai_agent_versions(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_conversations_owner_list ON ai_conversations (tenant_id, owner_user_id, status, last_message_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_conversations_tenant_list ON ai_conversations (tenant_id, visibility, status, last_message_at DESC);

CREATE TABLE IF NOT EXISTS ai_messages (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, conversation_id varchar(64) NOT NULL,
  role varchar(16) NOT NULL, status varchar(16) NOT NULL, sequence bigint NOT NULL,
  reply_to_message_id varchar(64), run_id varchar(64), stream_id varchar(64), author_user_id varchar(64),
  content jsonb NOT NULL, idempotency_key varchar(191) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), committed_at timestamptz,
  UNIQUE (tenant_id, conversation_id, sequence), UNIQUE (tenant_id, conversation_id, idempotency_key), UNIQUE (tenant_id, stream_id),
  CONSTRAINT ck_ai_messages_role CHECK (role IN ('user','assistant','system','tool')),
  CONSTRAINT ck_ai_messages_status CHECK (status IN ('pending','committed','failed','cancelled')),
  FOREIGN KEY (conversation_id) REFERENCES ai_conversations(id) ON DELETE RESTRICT,
  FOREIGN KEY (reply_to_message_id) REFERENCES ai_messages(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_messages_cursor ON ai_messages (tenant_id, conversation_id, sequence);

CREATE TABLE IF NOT EXISTS ai_assistant_streams (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, conversation_id varchar(64) NOT NULL,
  idempotency_key varchar(191) NOT NULL, status varchar(16) NOT NULL, message_id varchar(64),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
  UNIQUE (tenant_id, conversation_id, idempotency_key),
  CONSTRAINT ck_ai_stream_status CHECK (status IN ('streaming','committed','failed','cancelled')),
  FOREIGN KEY (conversation_id) REFERENCES ai_conversations(id) ON DELETE RESTRICT
);
CREATE TABLE IF NOT EXISTS ai_assistant_stream_deltas (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, stream_id varchar(64) NOT NULL,
  cursor bigint NOT NULL, type varchar(64) NOT NULL, data bytea NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, stream_id, cursor), FOREIGN KEY (stream_id) REFERENCES ai_assistant_streams(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_stream_deltas_replay ON ai_assistant_stream_deltas (tenant_id, stream_id, cursor);

CREATE TABLE IF NOT EXISTS ai_context_snapshots (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, conversation_id varchar(64) NOT NULL,
  conversation_revision bigint NOT NULL, agent_version_id varchar(64) NOT NULL,
  model_deployment_id varchar(64) NOT NULL, source_sequence_from bigint NOT NULL,
  source_sequence_to bigint NOT NULL, parent_snapshot_id varchar(64), kind varchar(16) NOT NULL,
  policy_version varchar(32) NOT NULL, token_estimate integer NOT NULL, status varchar(16) NOT NULL,
  payload jsonb NOT NULL, idempotency_key varchar(191) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, conversation_id, idempotency_key),
  CONSTRAINT ck_ai_snapshot_kind CHECK (kind IN ('raw','assembled')),
  CONSTRAINT ck_ai_snapshot_status CHECK (status IN ('ready','invalid')),
  CONSTRAINT ck_ai_snapshot_range CHECK (source_sequence_from <= source_sequence_to),
  FOREIGN KEY (conversation_id) REFERENCES ai_conversations(id) ON DELETE RESTRICT,
  FOREIGN KEY (agent_version_id) REFERENCES ai_agent_versions(id) ON DELETE RESTRICT,
  FOREIGN KEY (parent_snapshot_id) REFERENCES ai_context_snapshots(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_snapshots_conversation ON ai_context_snapshots (tenant_id, conversation_id, created_at DESC);
