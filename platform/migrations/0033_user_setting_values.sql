CREATE TABLE IF NOT EXISTS user_setting_values (
    id varchar(64) PRIMARY KEY,
    tenant_id varchar(64) NOT NULL,
    user_id varchar(64) NOT NULL,
    setting_key varchar(128) NOT NULL,
    schema_version integer NOT NULL,
    value_json jsonb NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    updated_by varchar(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT ux_user_setting_key UNIQUE (tenant_id, user_id, setting_key),
    CONSTRAINT fk_user_setting_owner FOREIGN KEY (tenant_id, user_id)
      REFERENCES users (tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_user_setting_values_owner
  ON user_setting_values (tenant_id, user_id);
