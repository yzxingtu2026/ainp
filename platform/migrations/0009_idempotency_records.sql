-- idempotency_records (Issue #22)
-- Apply manually in published environments; local/dev may rely on AutoMigrate + CHECKs.

CREATE TABLE IF NOT EXISTS idempotency_records (
    id               VARCHAR(64) PRIMARY KEY,
    tenant_id        VARCHAR(64) NOT NULL,
    user_id          VARCHAR(64) NOT NULL,
    action_id        VARCHAR(128) NOT NULL,
    idempotency_key  VARCHAR(191) NOT NULL,
    request_hash     VARCHAR(64) NOT NULL,
    status           VARCHAR(32) NOT NULL,
    response_json    JSONB,
    error_json       JSONB,
    started_at       TIMESTAMPTZ NOT NULL,
    completed_at     TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Composite unique: tenant + user + action + key ensures no cross-tenant/user/action key collision.
CREATE UNIQUE INDEX IF NOT EXISTS ux_idempotency_records_key
    ON idempotency_records (tenant_id, user_id, action_id, idempotency_key);

-- Status CHECK constraint.
ALTER TABLE idempotency_records DROP CONSTRAINT IF EXISTS ck_idempotency_records_status;
ALTER TABLE idempotency_records ADD CONSTRAINT ck_idempotency_records_status
    CHECK (status IN ('processing','completed','failed','conflict'));

-- Expiry index for cleanup.
CREATE INDEX IF NOT EXISTS idx_idempotency_records_expires
    ON idempotency_records (expires_at);
