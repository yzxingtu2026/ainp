-- Rename the framework-owned tenant-console account facts in place. Historical
-- migrations remain immutable because their checksums may already be recorded.
DO $$
BEGIN
  IF to_regclass('tenant_accounts') IS NULL AND to_regclass('users') IS NOT NULL
    AND to_regclass('user_roles') IS NOT NULL
    AND to_regclass('auth_identities') IS NOT NULL
    AND to_regclass('auth_sessions') IS NOT NULL
    AND EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'password_hash'
  ) AND EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'auth_version'
  ) AND EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema() AND table_name = 'user_roles' AND column_name = 'user_id'
  ) AND EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema() AND table_name = 'auth_identities' AND column_name = 'user_id'
  ) AND EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema() AND table_name = 'auth_sessions' AND column_name = 'user_id'
  ) THEN
    DROP INDEX IF EXISTS idx_users_username;
    ALTER TABLE users RENAME TO tenant_accounts;
  ELSIF to_regclass('tenant_accounts') IS NULL THEN
    RAISE EXCEPTION 'tenant_accounts is missing and users is not a legacy tenant-account table';
  END IF;

  IF to_regclass('tenant_account_roles') IS NULL AND to_regclass('user_roles') IS NOT NULL THEN
    ALTER TABLE user_roles RENAME TO tenant_account_roles;
  ELSIF to_regclass('tenant_account_roles') IS NULL THEN
    RAISE EXCEPTION 'neither user_roles nor tenant_account_roles exists';
  END IF;
END $$;

DO $$
DECLARE
  item record;
  old_exists boolean;
  new_exists boolean;
BEGIN
  FOR item IN
    SELECT * FROM (VALUES
      ('tenant_account_roles', 'user_id', 'account_id'),
      ('auth_identities', 'user_id', 'account_id'),
      ('auth_sessions', 'user_id', 'account_id')
    ) AS columns(table_name, old_name, new_name)
  LOOP
    SELECT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema = current_schema() AND table_name = item.table_name AND column_name = item.old_name
    ) INTO old_exists;
    SELECT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema = current_schema() AND table_name = item.table_name AND column_name = item.new_name
    ) INTO new_exists;

    IF old_exists AND new_exists THEN
      RAISE EXCEPTION 'both %.% and %.% exist; refusing ambiguous column migration',
        item.table_name, item.old_name, item.table_name, item.new_name;
    ELSIF old_exists THEN
      EXECUTE format('ALTER TABLE %I RENAME COLUMN %I TO %I', item.table_name, item.old_name, item.new_name);
    ELSIF NOT new_exists THEN
      RAISE EXCEPTION 'neither %.% nor %.% exists', item.table_name, item.old_name, item.table_name, item.new_name;
    END IF;
  END LOOP;
END $$;

DO $$
DECLARE
  item record;
  old_oid regclass;
  new_oid regclass;
BEGIN
  FOR item IN
    SELECT * FROM (VALUES
      ('ux_users_tenant_username', 'ux_tenant_accounts_tenant_username'),
      ('ux_users_tenant_id', 'ux_tenant_accounts_tenant_id'),
      ('idx_user_roles_tenant_id', 'idx_tenant_account_roles_tenant_id'),
      ('ux_user_role', 'ux_tenant_account_role'),
      ('idx_auth_identities_user', 'idx_auth_identities_account'),
      ('idx_auth_identities_user_id', 'idx_auth_identities_account_id'),
      ('idx_auth_sessions_owner', 'idx_auth_sessions_account')
    ) AS indexes(old_name, new_name)
  LOOP
    old_oid := to_regclass(item.old_name);
    new_oid := to_regclass(item.new_name);
    IF old_oid IS NOT NULL AND new_oid IS NOT NULL THEN
      RAISE EXCEPTION 'both indexes % and % exist; refusing ambiguous index migration', item.old_name, item.new_name;
    ELSIF old_oid IS NOT NULL THEN
      EXECUTE format('ALTER INDEX %I RENAME TO %I', item.old_name, item.new_name);
    END IF;
  END LOOP;
END $$;

DO $$
DECLARE
  item record;
  old_exists boolean;
  new_exists boolean;
BEGIN
  FOR item IN
    SELECT * FROM (VALUES
      ('tenant_accounts', 'users_pkey', 'tenant_accounts_pkey'),
      ('tenant_accounts', 'ck_users_auth_version', 'ck_tenant_accounts_auth_version'),
      ('tenant_accounts', 'ck_users_activation_status', 'ck_tenant_accounts_activation_status'),
      ('tenant_account_roles', 'user_roles_pkey', 'tenant_account_roles_pkey'),
      ('tenant_account_roles', 'user_roles_user_id_fkey', 'tenant_account_roles_account_id_fkey'),
      ('tenant_account_roles', 'user_roles_role_id_fkey', 'tenant_account_roles_role_id_fkey'),
      ('tenant_account_roles', 'fk_user_roles_user_tenant', 'fk_tenant_account_roles_account_tenant'),
      ('tenant_account_roles', 'fk_user_roles_role_tenant', 'fk_tenant_account_roles_role_tenant'),
      ('auth_identities', 'auth_identities_user_id_fkey', 'auth_identities_account_id_fkey'),
      ('auth_identities', 'fk_auth_identities_user_tenant', 'fk_auth_identities_account_tenant'),
      ('auth_sessions', 'auth_sessions_user_id_fkey', 'auth_sessions_account_id_fkey'),
      ('auth_sessions', 'fk_auth_sessions_user_tenant', 'fk_auth_sessions_account_tenant')
    ) AS constraints(table_name, old_name, new_name)
  LOOP
    SELECT EXISTS (
      SELECT 1 FROM pg_constraint
      WHERE conrelid = to_regclass(item.table_name) AND conname = item.old_name
    ) INTO old_exists;
    SELECT EXISTS (
      SELECT 1 FROM pg_constraint
      WHERE conrelid = to_regclass(item.table_name) AND conname = item.new_name
    ) INTO new_exists;

    IF old_exists AND new_exists THEN
      RAISE EXCEPTION 'both constraints % and % exist on %; refusing ambiguous constraint migration',
        item.old_name, item.new_name, item.table_name;
    ELSIF old_exists THEN
      EXECUTE format('ALTER TABLE %I RENAME CONSTRAINT %I TO %I', item.table_name, item.old_name, item.new_name);
    END IF;
  END LOOP;
END $$;
