-- attachments + attachment_links (Issue #18)
-- Apply manually in published environments; local/dev may rely on AutoMigrate + CHECKs.

CREATE TABLE IF NOT EXISTS attachments (
    id            VARCHAR(64) PRIMARY KEY,
    tenant_id     VARCHAR(64) NOT NULL,
    provider      VARCHAR(32) NOT NULL,
    object_key    VARCHAR(512) NOT NULL,
    original_name VARCHAR(512) NOT NULL,
    content_type  VARCHAR(128) NOT NULL,
    size          BIGINT NOT NULL DEFAULT 0,
    sha256        VARCHAR(64),
    status        VARCHAR(32) NOT NULL,
    uploader_id   VARCHAR(64) NOT NULL,
    upload_jti    VARCHAR(64),
    expected_size BIGINT NOT NULL DEFAULT 0,
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_attachments_tenant ON attachments (tenant_id);
CREATE INDEX IF NOT EXISTS idx_attachments_tenant_status ON attachments (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_attachments_deleted_at ON attachments (deleted_at);

ALTER TABLE attachments DROP CONSTRAINT IF EXISTS ck_attachments_status;
ALTER TABLE attachments ADD CONSTRAINT ck_attachments_status
    CHECK (status IN ('pending', 'ready', 'deleting', 'deleted', 'failed'));

CREATE TABLE IF NOT EXISTS attachment_links (
    id            VARCHAR(64) PRIMARY KEY,
    tenant_id     VARCHAR(64) NOT NULL,
    attachment_id VARCHAR(64) NOT NULL,
    resource_type VARCHAR(128) NOT NULL,
    resource_id   VARCHAR(128) NOT NULL,
    field         VARCHAR(64) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_attachment_link
    ON attachment_links (tenant_id, attachment_id, resource_type, resource_id, field);
CREATE INDEX IF NOT EXISTS idx_attachment_links_resource
    ON attachment_links (tenant_id, resource_type, resource_id, field);
