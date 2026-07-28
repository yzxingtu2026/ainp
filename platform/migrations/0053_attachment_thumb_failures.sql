-- Typed thumbnail failure metadata for retry and compensation decisions (Issue #200).
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS thumb_error_category VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS thumb_retryable BOOLEAN;

-- Historical failed rows remain NULL and are not automatically replayed.
CREATE INDEX IF NOT EXISTS idx_attachments_thumb_retry
    ON attachments (tenant_id, thumb_status, thumb_retryable);
