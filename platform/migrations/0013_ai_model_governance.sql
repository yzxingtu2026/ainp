CREATE TABLE IF NOT EXISTS ai_providers (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL REFERENCES tenants(id),
  code varchar(64) NOT NULL,
  name varchar(128) NOT NULL,
  protocol varchar(64) NOT NULL,
  base_url varchar(512) NOT NULL DEFAULT '',
  credential_ref varchar(160) NOT NULL,
  status varchar(32) NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled','disabled')),
  config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ux_ai_providers_tenant_code UNIQUE (tenant_id, code)
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_providers_tenant_id ON ai_providers(tenant_id,id);
CREATE TABLE IF NOT EXISTS ai_model_deployments (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL REFERENCES tenants(id),
  provider_id varchar(64) NOT NULL,
  code varchar(64) NOT NULL, remote_model_id varchar(255) NOT NULL, display_name varchar(128) NOT NULL,
  model_type varchar(32) NOT NULL CHECK (model_type IN ('chat','embedding','rerank','image','speech','transcription')),
  capabilities_json jsonb NOT NULL, limits_json jsonb NOT NULL, status varchar(32) NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled','disabled')),
  config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ux_ai_deployments_tenant_code UNIQUE (tenant_id, code),
  CONSTRAINT ux_ai_deployments_provider_remote UNIQUE (provider_id, remote_model_id),
  CONSTRAINT fk_ai_deployment_provider_tenant FOREIGN KEY (tenant_id, provider_id) REFERENCES ai_providers(tenant_id, id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_deployments_tenant_id ON ai_model_deployments(tenant_id,id);
CREATE TABLE IF NOT EXISTS ai_model_aliases (
  id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL REFERENCES tenants(id), code varchar(64) NOT NULL,
  deployment_id varchar(64) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ux_ai_aliases_tenant_code UNIQUE (tenant_id, code),
  CONSTRAINT fk_ai_alias_deployment_tenant FOREIGN KEY (tenant_id, deployment_id) REFERENCES ai_model_deployments(tenant_id, id) ON DELETE RESTRICT
);
