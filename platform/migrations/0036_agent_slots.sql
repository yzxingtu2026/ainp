CREATE TABLE IF NOT EXISTS ai_agent_slot_bindings (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, slot_key varchar(128) NOT NULL,
  slot_version varchar(32) NOT NULL, agent_id varchar(64) NOT NULL, revision bigint NOT NULL DEFAULT 1,
  created_by varchar(64) NOT NULL, updated_by varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ux_ai_agent_slot_binding UNIQUE (tenant_id, slot_key),
  CONSTRAINT fk_ai_agent_slot_binding_agent FOREIGN KEY (tenant_id, agent_id) REFERENCES ai_agents(tenant_id, id) ON DELETE RESTRICT
);
CREATE TABLE IF NOT EXISTS ai_agent_slot_preferences (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, user_id varchar(64) NOT NULL,
  slot_key varchar(128) NOT NULL, slot_version varchar(32) NOT NULL, agent_id varchar(64) NOT NULL,
  revision bigint NOT NULL DEFAULT 1, updated_by varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ux_ai_agent_slot_preference UNIQUE (tenant_id, user_id, slot_key),
  CONSTRAINT fk_ai_agent_slot_preference_user FOREIGN KEY (tenant_id, user_id) REFERENCES users(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT fk_ai_agent_slot_preference_agent FOREIGN KEY (tenant_id, agent_id) REFERENCES ai_agents(tenant_id, id) ON DELETE RESTRICT
);
