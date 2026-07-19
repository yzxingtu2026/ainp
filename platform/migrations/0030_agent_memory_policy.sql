ALTER TABLE ai_agents ADD COLUMN IF NOT EXISTS draft_memory_policy jsonb;
ALTER TABLE ai_agent_versions ADD COLUMN IF NOT EXISTS memory_policy jsonb NOT NULL DEFAULT '{}';
ALTER TABLE ai_agent_versions ADD COLUMN IF NOT EXISTS memory_policy_source varchar(32) NOT NULL DEFAULT 'system_default';
ALTER TABLE ai_agent_versions DROP CONSTRAINT IF EXISTS ck_ai_agent_memory_policy_source;
ALTER TABLE ai_agent_versions ADD CONSTRAINT ck_ai_agent_memory_policy_source CHECK (memory_policy_source IN ('agent_version','tenant_default','system_default'));
