-- Repair the page capture request tenant unique index (Issue #406).
-- AutoMigrate built ux_ai_page_capture_requests_tenant_id as a single-column
-- UNIQUE(tenant_id) index because the model's ID field was missing the
-- priority:2 tag; 0070's CREATE UNIQUE INDEX IF NOT EXISTS then skipped the
-- same-named index instead of fixing it. Recreate it as (tenant_id, id) so a
-- tenant can insert more than one capture request.

DROP INDEX IF EXISTS ux_ai_page_capture_requests_tenant_id;

CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_page_capture_requests_tenant_id
  ON ai_page_capture_requests (tenant_id, id);
