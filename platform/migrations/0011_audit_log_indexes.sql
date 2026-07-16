CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_occurred
    ON audit_logs (tenant_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_action_occurred
    ON audit_logs (tenant_id, action_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_resource
    ON audit_logs (tenant_id, resource_type, resource_id, occurred_at DESC);
