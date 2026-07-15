-- job_runs (Issue #21)
-- Apply manually in published environments; local/dev may rely on AutoMigrate + CHECKs.

CREATE TABLE IF NOT EXISTS job_runs (
    id                  VARCHAR(64) PRIMARY KEY,
    tenant_id           VARCHAR(64) NOT NULL,
    realm               VARCHAR(32) NOT NULL,
    job_type            VARCHAR(128) NOT NULL,
    schema_version      INTEGER NOT NULL,
    queue               VARCHAR(32) NOT NULL,
    payload_json        JSONB NOT NULL,
    status              VARCHAR(32) NOT NULL,
    progress            INTEGER NOT NULL DEFAULT 0,
    progress_message    VARCHAR(256),
    attempt             INTEGER NOT NULL DEFAULT 0,
    max_attempts        INTEGER NOT NULL DEFAULT 1,
    idempotency_key     VARCHAR(191),
    correlation_id      VARCHAR(64),
    causation_id        VARCHAR(64),
    created_by          VARCHAR(64),
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    cancel_requested_at TIMESTAMPTZ,
    last_error          VARCHAR(512),
    result_ref          VARCHAR(512),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_job_runs_tenant ON job_runs (tenant_id);
CREATE INDEX IF NOT EXISTS idx_job_runs_type ON job_runs (job_type);
CREATE INDEX IF NOT EXISTS idx_job_runs_status ON job_runs (status);

CREATE UNIQUE INDEX IF NOT EXISTS ux_job_runs_idem
    ON job_runs (tenant_id, job_type, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

ALTER TABLE job_runs DROP CONSTRAINT IF EXISTS ck_job_runs_status;
ALTER TABLE job_runs ADD CONSTRAINT ck_job_runs_status
    CHECK (status IN (
        'pending_enqueue', 'queued', 'running', 'retrying',
        'succeeded', 'failed', 'dead', 'cancelled'
    ));
