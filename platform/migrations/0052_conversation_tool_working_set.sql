ALTER TABLE ai_tool_scopes
    ADD COLUMN IF NOT EXISTS conversation_id varchar(64) REFERENCES ai_conversations(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_ai_tool_scopes_conversation
    ON ai_tool_scopes(tenant_id, conversation_id, expires_at DESC)
    WHERE conversation_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS ai_conversation_tool_working_set (
    id varchar(64) PRIMARY KEY,
    tenant_id varchar(64) NOT NULL REFERENCES tenants(id),
    conversation_id varchar(64) NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    tool_id varchar(255) NOT NULL,
    schema_hash varchar(64) NOT NULL,
    activated_at timestamptz NOT NULL,
    last_used_at timestamptz,
    use_count bigint NOT NULL DEFAULT 0 CHECK (use_count >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, conversation_id, tool_id)
);

CREATE INDEX IF NOT EXISTS idx_ai_conversation_tool_working_set_restore
    ON ai_conversation_tool_working_set(
        tenant_id,
        conversation_id,
        last_used_at DESC NULLS LAST,
        use_count DESC,
        activated_at DESC,
        tool_id ASC
    );
