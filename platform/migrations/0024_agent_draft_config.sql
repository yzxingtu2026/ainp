-- Builder 草稿配置：与 Agent 元数据一同保存，不晋升 current_version；正式发布后清空。
ALTER TABLE ai_agents
  ADD COLUMN IF NOT EXISTS draft_tenant_instructions text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS draft_model_alias varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS draft_generation_config jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS draft_tool_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS has_draft boolean NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_ai_agents_tenant_has_draft ON ai_agents (tenant_id) WHERE has_draft = true;
