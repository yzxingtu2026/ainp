-- Enforce the public account identity binding contract at the shared
-- auth_identities source of truth. Existing ambiguous data fails closed when
-- this unique index is created and must be reviewed before retrying migration.
CREATE UNIQUE INDEX ux_auth_identities_tenant_account_provider
  ON auth_identities (tenant_id, account_id, provider);
