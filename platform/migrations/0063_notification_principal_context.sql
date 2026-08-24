-- 0063_notification_principal_context.sql
-- Preserve immutable principal and client-surface facts across notifications.

ALTER TABLE notification_requests
  ADD COLUMN IF NOT EXISTS principal_kind varchar(128) NOT NULL DEFAULT 'ainp.account',
  ADD COLUMN IF NOT EXISTS client_surface varchar(128) NOT NULL DEFAULT 'ainp.tenant-console',
  ADD COLUMN IF NOT EXISTS audience varchar(255) NOT NULL DEFAULT 'ainp:tenant-console';

UPDATE notification_requests SET principal_kind = 'ainp.account'
 WHERE principal_kind IS NULL OR principal_kind = '';
UPDATE notification_requests SET client_surface = 'ainp.tenant-console'
 WHERE client_surface IS NULL OR client_surface = '';
UPDATE notification_requests SET audience = 'ainp:tenant-console'
 WHERE audience IS NULL OR audience = '';

CREATE INDEX IF NOT EXISTS idx_notification_requests_principal
  ON notification_requests (tenant_id, principal_kind, actor_id, client_surface, created_at DESC);

DROP INDEX IF EXISTS uq_notification_requests_idem;
DROP INDEX IF EXISTS uq_notif_req_idem;
CREATE UNIQUE INDEX uq_notif_req_idem
  ON notification_requests (tenant_id, definition_key, principal_kind, actor_id, client_surface, audience, idempotency_key)
  WHERE idempotency_key != '';
