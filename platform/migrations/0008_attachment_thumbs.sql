-- Attachment thumb derivative columns (Issue #36)
-- Apply manually in published environments; local/dev may rely on AutoMigrate + CHECKs.

ALTER TABLE attachments ADD COLUMN IF NOT EXISTS thumb_status VARCHAR(32);
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS thumb_object_key VARCHAR(512);
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS thumb_content_type VARCHAR(128);
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS thumb_size BIGINT NOT NULL DEFAULT 0;
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS thumb_error VARCHAR(512);
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS thumb_updated_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_attachments_thumb_status ON attachments (tenant_id, thumb_status);

ALTER TABLE attachments DROP CONSTRAINT IF EXISTS ck_attachments_thumb_status;
ALTER TABLE attachments ADD CONSTRAINT ck_attachments_thumb_status
    CHECK (thumb_status IS NULL OR thumb_status IN ('pending', 'ready', 'failed', 'skipped'));
