-- Canonicalize tenant-console identity contracts without rewriting immutable
-- historical migrations. The migration is atomic under the migration runner.

WITH permission_map(old_permission, new_permission) AS (
  VALUES
    ('system.user_list', 'system.account_list'),
    ('system.user_get', 'system.account_get'),
    ('system.user_create', 'system.account_create'),
    ('system.user_update', 'system.account_update'),
    ('system.user_status_update', 'system.account_status_update'),
    ('system.user_roles_update', 'system.account_roles_update'),
    ('system.user_password_reset', 'system.account_password_reset')
)
DELETE FROM role_permissions old
USING permission_map mapping
WHERE old.permission = mapping.old_permission
  AND EXISTS (
    SELECT 1 FROM role_permissions canonical
    WHERE canonical.role_id = old.role_id
      AND canonical.permission = mapping.new_permission
  );

WITH permission_map(old_permission, new_permission) AS (
  VALUES
    ('system.user_list', 'system.account_list'),
    ('system.user_get', 'system.account_get'),
    ('system.user_create', 'system.account_create'),
    ('system.user_update', 'system.account_update'),
    ('system.user_status_update', 'system.account_status_update'),
    ('system.user_roles_update', 'system.account_roles_update'),
    ('system.user_password_reset', 'system.account_password_reset')
)
UPDATE role_permissions role_permission
SET permission = mapping.new_permission
FROM permission_map mapping
WHERE role_permission.permission = mapping.old_permission;

DO $$
DECLARE
  target record;
  old_exists boolean;
  new_exists boolean;
  old_not_null boolean;
  conflicting_rows bigint;
BEGIN
  FOR target IN
    SELECT * FROM (VALUES
      ('audit_logs', 'user_id', 'actor_id'),
      ('idempotency_records', 'user_id', 'principal_id'),
      ('ai_tool_scopes', 'user_id', 'principal_id'),
      ('ai_tool_confirmations', 'user_id', 'principal_id'),
      ('ai_tool_invocations', 'user_id', 'principal_id'),
      ('mcp_oauth_grants', 'user_id', 'account_id'),
      ('mcp_action_confirmations', 'user_id', 'account_id'),
      ('ai_knowledge_bases', 'owner_user_id', 'account_id'),
      ('user_setting_values', 'user_id', 'account_id'),
      ('ai_assistant_personalization_snapshots', 'user_id', 'account_id'),
      ('user_notifications', 'user_id', 'account_id')
    ) AS mapping(table_name, old_column, new_column)
  LOOP
    SELECT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema = current_schema()
        AND table_name = target.table_name
        AND column_name = target.old_column
    ) INTO old_exists;
    SELECT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema = current_schema()
        AND table_name = target.table_name
        AND column_name = target.new_column
    ) INTO new_exists;

    IF old_exists AND new_exists THEN
      EXECUTE format(
        'SELECT count(*) FROM %I WHERE NULLIF(%I, '''') IS NOT NULL AND NULLIF(%I, '''') IS NOT NULL AND %I IS DISTINCT FROM %I',
        target.table_name, target.old_column, target.new_column, target.old_column, target.new_column
      ) INTO conflicting_rows;
      IF conflicting_rows > 0 THEN
        RAISE EXCEPTION 'conflicting identity values in %.% and %.% (% rows)',
          target.table_name, target.old_column, target.table_name, target.new_column, conflicting_rows;
      END IF;

      SELECT is_nullable = 'NO'
      FROM information_schema.columns
      WHERE table_schema = current_schema()
        AND table_name = target.table_name
        AND column_name = target.old_column
      INTO old_not_null;

      EXECUTE format(
        'UPDATE %I SET %I = %I WHERE NULLIF(%I, '''') IS NULL AND NULLIF(%I, '''') IS NOT NULL',
        target.table_name, target.new_column, target.old_column, target.new_column, target.old_column
      );
      IF old_not_null THEN
        EXECUTE format('ALTER TABLE %I ALTER COLUMN %I SET NOT NULL', target.table_name, target.new_column);
      END IF;
      EXECUTE format('ALTER TABLE %I DROP COLUMN %I', target.table_name, target.old_column);
    ELSIF old_exists THEN
      EXECUTE format('ALTER TABLE %I RENAME COLUMN %I TO %I', target.table_name, target.old_column, target.new_column);
    ELSIF NOT new_exists THEN
      RAISE EXCEPTION 'neither %.% nor %.% exists',
        target.table_name, target.old_column, target.table_name, target.new_column;
    END IF;
  END LOOP;
END $$;

DO $$
DECLARE
  target record;
BEGIN
  FOR target IN
    SELECT * FROM (VALUES
      ('idx_audit_logs_user_id', 'idx_audit_logs_actor_id'),
      ('idx_ai_tool_scopes_actor', 'idx_ai_tool_scopes_principal'),
      ('idx_ai_tool_confirmations_actor_status', 'idx_ai_tool_confirmations_principal_status'),
      ('idx_ai_tool_invocations_actor_started', 'idx_ai_tool_invocations_principal_started'),
      ('idx_ai_tool_scopes_user_id', 'idx_ai_tool_scopes_principal_id'),
      ('idx_ai_tool_confirmations_user_id', 'idx_ai_tool_confirmations_principal_id'),
      ('idx_ai_tool_invocations_user_id', 'idx_ai_tool_invocations_principal_id'),
      ('idx_mcp_oauth_grants_user_id', 'idx_mcp_oauth_grants_account_id'),
      ('idx_mcp_action_confirmations_user_id', 'idx_mcp_action_confirmations_account_id'),
      ('idx_ai_knowledge_bases_owner_user_id', 'idx_ai_knowledge_bases_account_id'),
      ('idx_user_setting_values_user_id', 'idx_user_setting_values_account_id'),
      ('idx_user_notifications_user_unread', 'idx_user_notifications_account_unread'),
      ('idx_user_notifications_user', 'idx_user_notifications_account'),
      ('idx_user_notifications_user_archived', 'idx_user_notifications_account_archived'),
      ('idx_user_notif_user', 'idx_user_notif_account'),
      ('uq_user_notif_delivery_user', 'uq_user_notif_delivery_account')
    ) AS mapping(old_index, new_index)
  LOOP
    IF to_regclass(target.old_index) IS NOT NULL AND to_regclass(target.new_index) IS NOT NULL THEN
      RAISE EXCEPTION 'both identity indexes % and % exist; refusing ambiguous identity migration', target.old_index, target.new_index;
    ELSIF to_regclass(target.old_index) IS NOT NULL THEN
      EXECUTE format('ALTER INDEX %I RENAME TO %I', target.old_index, target.new_index);
    END IF;
  END LOOP;
END $$;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'user_notifications'::regclass
      AND conname = 'uq_user_notifications_delivery_user'
  ) AND EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'user_notifications'::regclass
      AND conname = 'uq_user_notifications_delivery_account'
  ) THEN
    RAISE EXCEPTION 'both user notification account constraints exist; refusing ambiguous identity migration';
  ELSIF EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'user_notifications'::regclass
      AND conname = 'uq_user_notifications_delivery_user'
  ) THEN
    ALTER TABLE user_notifications
      RENAME CONSTRAINT uq_user_notifications_delivery_user TO uq_user_notifications_delivery_account;
  END IF;
END $$;

DELETE FROM attachment_links legacy
WHERE legacy.resource_type = 'system.user_profile'
  AND EXISTS (
    SELECT 1 FROM attachment_links canonical
    WHERE canonical.tenant_id = legacy.tenant_id
      AND canonical.attachment_id = legacy.attachment_id
      AND canonical.resource_type = 'system.account_profile'
      AND canonical.resource_id = legacy.resource_id
      AND canonical.field = legacy.field
  );

UPDATE attachment_links
SET resource_type = 'system.account_profile'
WHERE resource_type = 'system.user_profile';
