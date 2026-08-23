package outbox

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DispatcherConfig controls polling and claim behavior.
type DispatcherConfig struct {
	Enabled         bool
	BatchSize       int
	PollInterval    time.Duration
	LockTimeout     time.Duration
	MaxAttempts     int
	Backoff         BackoffConfig
	ShutdownTimeout time.Duration
	WorkerID        string
}

func (c DispatcherConfig) withDefaults() DispatcherConfig {
	if c.BatchSize <= 0 {
		c.BatchSize = 32
	}
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.LockTimeout <= 0 {
		c.LockTimeout = 30 * time.Second
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 10
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 15 * time.Second
	}
	if c.WorkerID == "" {
		c.WorkerID = "outbox-" + uuid.NewString()[:8]
	}
	c.Backoff = c.Backoff.withDefaults()
	return c
}

// Dispatcher claims pending outbox rows and publishes them via Transport.
type Dispatcher struct {
	db        *gorm.DB
	transport Transport
	cfg       DispatcherConfig
	listener  NotificationListener
	wake      chan struct{}
	listenWG  sync.WaitGroup

	cancel context.CancelFunc
	done   chan struct{}
}

func NewDispatcher(db *gorm.DB, transport Transport, cfg DispatcherConfig) *Dispatcher {
	return &Dispatcher{
		db:        db,
		transport: transport,
		cfg:       cfg.withDefaults(),
		done:      make(chan struct{}),
		wake:      make(chan struct{}, 1),
	}
}

func (d *Dispatcher) SetNotificationListener(listener NotificationListener) {
	d.listener = listener
}

func (d *Dispatcher) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	d.cancel = cancel
	go func() {
		defer close(d.done)
		if d.listener != nil {
			d.listenWG.Add(1)
			go func() {
				defer d.listenWG.Done()
				d.listen(ctx)
			}()
		}
		d.loop(ctx)
		d.listenWG.Wait()
	}()
}

func (d *Dispatcher) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	select {
	case <-d.done:
	case <-time.After(d.cfg.ShutdownTimeout):
		logging.Warn(context.Background(), "outbox dispatcher shutdown timed out",
			"worker_id", d.cfg.WorkerID,
		)
	}
}

func (d *Dispatcher) loop(ctx context.Context) {
	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()
	for {
		if err := d.Tick(ctx); err != nil && ctx.Err() == nil {
			logging.Error(ctx, "outbox dispatcher tick failed", err, "worker_id", d.cfg.WorkerID)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-d.wake:
		}
	}
}

func (d *Dispatcher) listen(ctx context.Context) {
	backoff := 250 * time.Millisecond
	for ctx.Err() == nil {
		err := d.listener.Listen(ctx, d.Wake)
		if ctx.Err() != nil {
			return
		}
		logging.Warn(ctx, "outbox notification listener disconnected", "worker_id", d.cfg.WorkerID, "retry_in_ms", backoff.Milliseconds(), "error", truncateError(err))
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

// Wake non-blockingly coalesces commit notifications. The main loop remains the
// only Tick caller, so notification storms cannot create overlapping claims.
func (d *Dispatcher) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Tick recovers stale locks and processes one batch. Exported for tests.
func (d *Dispatcher) Tick(ctx context.Context) error {
	if d.db == nil || d.transport == nil {
		return fmt.Errorf("outbox: dispatcher not configured")
	}
	if err := d.recoverStaleLocks(ctx); err != nil {
		return err
	}
	for {
		events, err := d.claimBatch(ctx)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			d.processOne(ctx, ev)
		}
		if len(events) < d.cfg.BatchSize {
			return nil
		}
	}
}

func (d *Dispatcher) recoverStaleLocks(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-d.cfg.LockTimeout)
	return d.db.WithContext(ctx).Model(&Event{}).
		Where("status = ? AND locked_at IS NOT NULL AND locked_at < ?", status.OutboxProcessing, cutoff).
		Updates(map[string]any{
			"status":     status.OutboxPending,
			"locked_by":  "",
			"locked_at":  nil,
			"updated_at": time.Now().UTC(),
		}).Error
}

func (d *Dispatcher) claimBatch(ctx context.Context) ([]Event, error) {
	var claimed []Event
	now := time.Now().UTC()
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []Event
		q := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND available_at <= ?", []status.OutboxStatus{status.OutboxPending, status.OutboxFailed}, now).
			Order("available_at ASC").
			Limit(d.cfg.BatchSize).
			Find(&rows)
		if q.Error != nil {
			return q.Error
		}
		for _, row := range rows {
			res := tx.Model(&Event{}).
				Where("id = ? AND status IN ?", row.ID, []status.OutboxStatus{status.OutboxPending, status.OutboxFailed}).
				Updates(map[string]any{
					"status":     status.OutboxProcessing,
					"locked_by":  d.cfg.WorkerID,
					"locked_at":  now,
					"updated_at": now,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 1 {
				row.Status = status.OutboxProcessing
				row.LockedBy = d.cfg.WorkerID
				row.LockedAt = &now
				claimed = append(claimed, row)
			}
		}
		return nil
	})
	return claimed, err
}

func (d *Dispatcher) processOne(ctx context.Context, ev Event) {
	traceID := ev.CorrelationID
	if traceID == "" {
		traceID = ev.ID
	}
	ctx = auth.WithContext(ctx, &auth.Context{RequestID: uuid.NewString(), TraceID: traceID, TenantID: ev.TenantID, Realm: auth.Realm(ev.Realm), PrincipalID: ev.ActorID, Principal: auth.PrincipalRef{Kind: auth.PrincipalKind(ev.PrincipalKind), ID: ev.ActorID}, ClientSurface: ev.ClientSurface, Audience: ev.Audience, Client: "system", Authenticated: true})
	start := time.Now()
	env := rowToEnvelope(ev)
	err := d.transport.Publish(ctx, env)
	dur := time.Since(start)
	if err != nil {
		attrs := []any{
			"event_id", ev.ID,
			"event_type", ev.EventType,
			"tenant_id", ev.TenantID,
			"transport", d.transport.Name(),
			"attempt", ev.Attempts + 1,
			"duration_ms", dur.Milliseconds(),
			"error", truncateError(err),
			"error_category", "internal", "error_stage", "outbox.publish", "safe_summary", "outbox publish failed",
		}
		if ev.Attempts+1 >= d.cfg.MaxAttempts {
			logging.Error(ctx, "outbox publish failed", err, attrs...)
		} else {
			logging.Warn(ctx, "outbox publish failed", attrs...)
		}
		d.markFailure(ctx, ev, err)
		return
	}
	now := time.Now().UTC()
	if err := d.db.WithContext(ctx).Model(&Event{}).Where("id = ?", ev.ID).Updates(map[string]any{
		"status":       status.OutboxPublished,
		"published_at": now,
		"locked_by":    "",
		"locked_at":    nil,
		"last_error":   "",
		"updated_at":   now,
		"attempts":     ev.Attempts + 1,
	}).Error; err != nil {
		logging.Error(ctx, "outbox mark published failed", err, "event_id", ev.ID)
		return
	}
	logging.Info(ctx, "outbox published",
		"event_id", ev.ID,
		"event_type", ev.EventType,
		"tenant_id", ev.TenantID,
		"transport", d.transport.Name(),
		"attempt", ev.Attempts+1,
		"duration_ms", dur.Milliseconds(),
		"outbox_publish_latency_ms", now.Sub(ev.OccurredAt).Milliseconds(),
	)
}

func (d *Dispatcher) markFailure(ctx context.Context, ev Event, publishErr error) {
	attempts := ev.Attempts + 1
	now := time.Now().UTC()
	nextStatus := status.OutboxFailed
	availableAt := now.Add(d.cfg.Backoff.NextDelay(attempts))
	if attempts >= d.cfg.MaxAttempts {
		nextStatus = status.OutboxDead
		availableAt = now
	}
	_ = d.db.WithContext(ctx).Model(&Event{}).Where("id = ?", ev.ID).Updates(map[string]any{
		"status":       nextStatus,
		"attempts":     attempts,
		"available_at": availableAt,
		"locked_by":    "",
		"locked_at":    nil,
		"last_error":   truncateError(publishErr),
		"updated_at":   now,
	}).Error
}

// RetryEvent resets a failed/dead event to pending for manual retry (no payload mutation).
func RetryEvent(ctx context.Context, db *gorm.DB, eventID string) error {
	now := time.Now().UTC()
	res := db.WithContext(ctx).Model(&Event{}).
		Where("id = ? AND status IN ?", eventID, []status.OutboxStatus{status.OutboxFailed, status.OutboxDead}).
		Updates(map[string]any{
			"status":       status.OutboxPending,
			"available_at": now,
			"locked_by":    "",
			"locked_at":    nil,
			"last_error":   "",
			"updated_at":   now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("outbox: event %s not found or not retryable", eventID)
	}
	return nil
}

// RetryTenantEvent resets a current-tenant failed/dead event to pending.
func RetryTenantEvent(ctx context.Context, db *gorm.DB, eventID string) error {
	tdb, err := tenant.TenantDB(ctx, db)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	rows, err := tenant.TenantRaw[struct{ ID string }](tdb, `WITH scoped AS (
  SELECT id FROM outbox_events WHERE tenant_id = ? AND id = ? AND status IN (?, ?) FOR UPDATE
)
UPDATE outbox_events AS e
SET status = ?, available_at = ?, locked_by = '', locked_at = NULL, last_error = '', updated_at = ?
FROM scoped WHERE e.id = scoped.id
RETURNING e.id`, eventID, status.OutboxFailed, status.OutboxDead, status.OutboxPending, now, now)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("outbox: event %s not found or not retryable", eventID)
	}
	return nil
}
