CREATE TABLE IF NOT EXISTS tenant_setting_values (
    id varchar(64) PRIMARY KEY,
    tenant_id varchar(64) NOT NULL,
    setting_key varchar(128) NOT NULL,
    schema_version integer NOT NULL,
    value_json jsonb NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    updated_by varchar(64) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT ux_tenant_setting_key UNIQUE (tenant_id, setting_key)
);

CREATE INDEX IF NOT EXISTS idx_tenant_setting_values_tenant_id ON tenant_setting_values (tenant_id);
