ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS mcp_client_id varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS mcp_grant_id varchar(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_mcp_client_id
    ON audit_logs (tenant_id, mcp_client_id)
    WHERE mcp_client_id <> '';

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_mcp_grant_id
    ON audit_logs (tenant_id, mcp_grant_id)
    WHERE mcp_grant_id <> '';
