CREATE TABLE IF NOT EXISTS ai_agents (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, owner_user_id varchar(64),
  contract_key varchar(128), contract_version varchar(32), code varchar(64) NOT NULL,
  name varchar(128) NOT NULL, description varchar(512) NOT NULL DEFAULT '', avatar_attachment_id varchar(64),
  source varchar(16) NOT NULL, visibility varchar(16) NOT NULL, status varchar(16) NOT NULL,
  current_version integer NOT NULL DEFAULT 0, revision bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ux_ai_agents_tenant_code UNIQUE (tenant_id, code),
  CONSTRAINT ck_ai_agents_source CHECK (source IN ('builtin','tenant','personal')),
  CONSTRAINT ck_ai_agents_visibility CHECK (visibility IN ('private','tenant')),
  CONSTRAINT ck_ai_agents_status CHECK (status IN ('draft','enabled','disabled')),
  CONSTRAINT ck_ai_agents_contract CHECK ((source = 'builtin' AND contract_key <> '' AND contract_version <> '') OR source <> 'builtin')
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_agents_builtin_contract ON ai_agents (tenant_id, contract_key) WHERE source = 'builtin';
CREATE INDEX IF NOT EXISTS idx_ai_agents_tenant_status ON ai_agents (tenant_id, status);

CREATE TABLE IF NOT EXISTS ai_agent_versions (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, agent_id varchar(64) NOT NULL,
  version integer NOT NULL, model_alias varchar(64) NOT NULL, tenant_instructions text NOT NULL DEFAULT '',
  user_instructions text NOT NULL DEFAULT '', generation_config jsonb NOT NULL DEFAULT '{}'::jsonb,
  tool_policy jsonb NOT NULL DEFAULT '{}'::jsonb, created_by varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE (tenant_id, agent_id, version),
  FOREIGN KEY (agent_id) REFERENCES ai_agents(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_ai_agent_versions_agent ON ai_agent_versions (tenant_id, agent_id, version DESC);

CREATE TABLE IF NOT EXISTS ai_agent_tool_bindings (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, agent_id varchar(64) NOT NULL,
  tool_id varchar(255) NOT NULL, source varchar(32) NOT NULL, status varchar(16) NOT NULL,
  created_by varchar(64) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, agent_id, tool_id), FOREIGN KEY (agent_id) REFERENCES ai_agents(id) ON DELETE RESTRICT,
  CONSTRAINT ck_ai_agent_tool_status CHECK (status IN ('enabled','disabled'))
);
