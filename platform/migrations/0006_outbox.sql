-- outbox_events + event_deliveries (Issue #20)
-- Apply manually in published environments; local/dev may rely on AutoMigrate + CHECKs.

CREATE TABLE IF NOT EXISTS outbox_events (
    id                VARCHAR(64) PRIMARY KEY,
    tenant_id         VARCHAR(64) NOT NULL,
    realm             VARCHAR(32) NOT NULL,
    event_type        VARCHAR(128) NOT NULL,
    schema_version    INTEGER NOT NULL,
    aggregate_type    VARCHAR(128) NOT NULL DEFAULT '',
    aggregate_id      VARCHAR(128) NOT NULL DEFAULT '',
    aggregate_version BIGINT NOT NULL DEFAULT 0,
    payload_json      JSONB NOT NULL,
    metadata_json     JSONB,
    status            VARCHAR(32) NOT NULL,
    attempts          INTEGER NOT NULL DEFAULT 0,
    available_at      TIMESTAMPTZ NOT NULL,
    locked_by         VARCHAR(128),
    locked_at         TIMESTAMPTZ,
    occurred_at       TIMESTAMPTZ NOT NULL,
    published_at      TIMESTAMPTZ,
    correlation_id    VARCHAR(64),
    causation_id      VARCHAR(64),
    actor_id          VARCHAR(64),
    last_error        VARCHAR(512),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbox_tenant ON outbox_events (tenant_id);
CREATE INDEX IF NOT EXISTS idx_outbox_event_type ON outbox_events (event_type);
CREATE INDEX IF NOT EXISTS idx_outbox_aggregate ON outbox_events (aggregate_type, aggregate_id);
CREATE INDEX IF NOT EXISTS idx_outbox_claim ON outbox_events (status, available_at);

ALTER TABLE outbox_events DROP CONSTRAINT IF EXISTS ck_outbox_events_status;
ALTER TABLE outbox_events ADD CONSTRAINT ck_outbox_events_status
    CHECK (status IN ('pending', 'processing', 'published', 'failed', 'dead'));

CREATE TABLE IF NOT EXISTS event_deliveries (
    id           VARCHAR(64) PRIMARY KEY,
    event_id     VARCHAR(64) NOT NULL,
    handler_name VARCHAR(128) NOT NULL,
    status       VARCHAR(32) NOT NULL,
    attempts     INTEGER NOT NULL DEFAULT 0,
    processed_at TIMESTAMPTZ,
    last_error   VARCHAR(512),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_event_handler ON event_deliveries (event_id, handler_name);

ALTER TABLE event_deliveries DROP CONSTRAINT IF EXISTS ck_event_deliveries_status;
ALTER TABLE event_deliveries ADD CONSTRAINT ck_event_deliveries_status
    CHECK (status IN ('pending', 'processing', 'succeeded', 'failed'));
