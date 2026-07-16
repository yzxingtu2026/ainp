-- 0010_notifications.sql
-- Notification system: requests, deliveries, template overrides, user notifications.

CREATE TABLE IF NOT EXISTS notification_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    definition_key  TEXT NOT NULL,
    category        TEXT NOT NULL DEFAULT '',
    priority        TEXT NOT NULL DEFAULT 'normal',
    idempotency_key TEXT NOT NULL DEFAULT '',
    variables_json  JSONB NOT NULL DEFAULT '{}',
    actor_id        TEXT NOT NULL DEFAULT '',
    correlation_id  TEXT NOT NULL DEFAULT '',
    causation_id    TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_notification_requests_tenant CHECK (tenant_id != '00000000-0000-0000-0000-000000000000')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notification_requests_idem
    ON notification_requests (tenant_id, definition_key, idempotency_key)
    WHERE idempotency_key != '';

CREATE INDEX IF NOT EXISTS idx_notification_requests_tenant
    ON notification_requests (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notification_requests_actor
    ON notification_requests (tenant_id, actor_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL,
    request_id        UUID NOT NULL REFERENCES notification_requests(id) ON DELETE CASCADE,
    recipient_type    TEXT NOT NULL DEFAULT 'user',
    recipient_id      TEXT NOT NULL,
    channel           TEXT NOT NULL,
    provider          TEXT NOT NULL DEFAULT '',
    template_version  INT NOT NULL DEFAULT 0,
    locale            TEXT NOT NULL DEFAULT 'en',
    status            TEXT NOT NULL DEFAULT 'pending',
    attempts          INT NOT NULL DEFAULT 0,
    max_attempts      INT NOT NULL DEFAULT 3,
    external_id       TEXT NOT NULL DEFAULT '',
    scheduled_at      TIMESTAMPTZ,
    sent_at           TIMESTAMPTZ,
    delivered_at      TIMESTAMPTZ,
    failed_at         TIMESTAMPTZ,
    last_error        TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_notification_deliveries_status CHECK (
        status IN ('pending','queued','sending','sent','delivered','failed','dead','cancelled','skipped')
    )
);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_tenant
    ON notification_deliveries (tenant_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_request
    ON notification_deliveries (request_id);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_recipient
    ON notification_deliveries (tenant_id, recipient_id, channel, status);

CREATE TABLE IF NOT EXISTS notification_template_overrides (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    definition_key  TEXT NOT NULL,
    channel         TEXT NOT NULL,
    locale          TEXT NOT NULL,
    version         INT NOT NULL DEFAULT 1,
    subject         TEXT NOT NULL DEFAULT '',
    body            TEXT NOT NULL DEFAULT '',
    config_json     JSONB NOT NULL DEFAULT '{}',
    enabled         BOOLEAN NOT NULL DEFAULT true,
    updated_by      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_notification_template_overrides UNIQUE (tenant_id, definition_key, channel, locale)
);

CREATE INDEX IF NOT EXISTS idx_notification_template_overrides_tenant
    ON notification_template_overrides (tenant_id);

CREATE TABLE IF NOT EXISTS user_notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    user_id     TEXT NOT NULL,
    request_id  UUID NOT NULL REFERENCES notification_requests(id) ON DELETE CASCADE,
    delivery_id UUID NOT NULL REFERENCES notification_deliveries(id) ON DELETE CASCADE,
    title       TEXT NOT NULL DEFAULT '',
    summary     TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL DEFAULT '',
    icon        TEXT NOT NULL DEFAULT '',
    local_icon  TEXT NOT NULL DEFAULT '',
    action_json JSONB NOT NULL DEFAULT '{}',
    read_at     TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_user_notifications_delivery_user UNIQUE (delivery_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_notifications_user_unread
    ON user_notifications (tenant_id, user_id, read_at)
    WHERE read_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_user_notifications_user
    ON user_notifications (tenant_id, user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_user_notifications_user_archived
    ON user_notifications (tenant_id, user_id, archived_at)
    WHERE archived_at IS NULL;
