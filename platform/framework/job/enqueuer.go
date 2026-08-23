package job

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"gorm.io/gorm"
)

// EnqueueInput is the business-facing enqueue request.
type EnqueueInput struct {
	Type           string
	Payload        any
	IdempotencyKey string
	CausationID    string
	ProcessIn      time.Duration
}

// Enqueuer creates JobRun facts and enqueues via Transport.
type Enqueuer interface {
	Enqueue(ctx context.Context, in EnqueueInput) (*Run, error)
}

type enqueuer struct {
	db         *gorm.DB
	registry   *Registry
	store      RunStore
	transport  Transport
	enabled    bool
	maxPayload int
}

func NewEnqueuer(db *gorm.DB, registry *Registry, store RunStore, transport Transport, enabled bool, maxPayload int) Enqueuer {
	if maxPayload <= 0 {
		maxPayload = 256 * 1024
	}
	return &enqueuer{
		db: db, registry: registry, store: store, transport: transport,
		enabled: enabled, maxPayload: maxPayload,
	}
}

func (e *enqueuer) Enqueue(ctx context.Context, in EnqueueInput) (*Run, error) {
	if !e.enabled || e.transport == nil {
		return nil, ErrUnavailable
	}
	def, ok := e.registry.Definition(in.Type)
	if !ok {
		return nil, Permanent(fmt.Errorf("unknown job type %q", in.Type))
	}
	tenantID, realm, principal, surface, audience, correlationID, err := ResolveIdentityContext(ctx)
	if err != nil {
		return nil, Permanent(err)
	}
	payload, err := MarshalPayload(in.Payload)
	if err != nil {
		return nil, Permanent(err)
	}
	limit := def.MaxPayloadSize
	if limit <= 0 {
		limit = e.maxPayload
	}
	if len(payload) > limit {
		return nil, Permanent(fmt.Errorf("payload exceeds limit %d", limit))
	}

	now := time.Now().UTC()
	run := &Run{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		Realm:          string(auth.NormalizeRealm(realm)),
		JobType:        def.Type,
		SchemaVersion:  def.SchemaVersion,
		Queue:          def.Queue,
		PayloadJSON:    payload,
		Status:         status.JobPendingEnqueue,
		Progress:       0,
		Attempt:        0,
		MaxAttempts:    def.MaxAttempts,
		IdempotencyKey: strings.TrimSpace(in.IdempotencyKey),
		CorrelationID:  correlationID,
		CausationID:    firstNonEmpty(in.CausationID),
		CreatedBy:      principal.ID,
		PrincipalKind:  string(principal.Kind),
		ClientSurface:  surface,
		Audience:       audience,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	existing, created, err := CreateIdempotent(ctx, e.db, run)
	if err != nil {
		return nil, Retryable(err)
	}
	if !created {
		if existing.Status == status.JobPendingEnqueue {
			def2, ok := e.registry.Definition(existing.JobType)
			if !ok {
				return existing, Permanent(fmt.Errorf("unknown job type %q", existing.JobType))
			}
			if err := e.dispatch(ctx, existing, def2, in.ProcessIn); err != nil {
				_ = e.store.UpdateFields(ctx, existing.ID, map[string]any{
					"last_error": TruncateError(err),
					"status":     status.JobPendingEnqueue,
				})
				return existing, Retryable(err)
			}
			existing.Status = status.JobQueued
		}
		return existing, nil
	}
	if err := e.dispatch(ctx, existing, def, in.ProcessIn); err != nil {
		_ = e.store.UpdateFields(ctx, existing.ID, map[string]any{
			"last_error": TruncateError(err),
			"status":     status.JobPendingEnqueue,
		})
		return existing, Retryable(err)
	}
	return existing, nil
}

func (e *enqueuer) dispatch(ctx context.Context, run *Run, def Definition, processIn time.Duration) error {
	env := Envelope{
		ID:            run.ID,
		Type:          run.JobType,
		SchemaVersion: run.SchemaVersion,
		TenantID:      run.TenantID,
		Realm:         auth.Realm(run.Realm),
		ActorID:       run.CreatedBy,
		Principal:     auth.PrincipalRef{Kind: auth.PrincipalKind(run.PrincipalKind), ID: run.CreatedBy},
		ClientSurface: run.ClientSurface,
		Audience:      run.Audience,
		CorrelationID: run.CorrelationID,
		CausationID:   run.CausationID,
		CreatedAt:     run.CreatedAt,
		Attempt:       run.Attempt,
	}
	opts := EnqueueTransportOpts{
		TaskID:    run.ID,
		MaxRetry:  def.MaxAttempts - 1, // asynq retries = maxAttempts-1 additional tries
		Timeout:   def.Timeout,
		ProcessIn: processIn,
		UniqueFor: def.UniqueFor,
	}
	if err := e.transport.Enqueue(ctx, env, def.Queue, opts); err != nil {
		return err
	}
	return e.store.UpdateFields(ctx, run.ID, map[string]any{
		"status":     status.JobQueued,
		"last_error": "",
	})
}

// RecoverPendingEnqueue re-dispatches stuck pending_enqueue runs (crash window recovery).
func RecoverPendingEnqueue(ctx context.Context, e *enqueuer, limit int) (int, error) {
	if e == nil || !e.enabled {
		return 0, nil
	}
	rows, err := e.store.ListPendingEnqueue(ctx, limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, run := range rows {
		def, ok := e.registry.Definition(run.JobType)
		if !ok {
			continue
		}
		r := run
		if err := e.dispatch(ctx, &r, def, 0); err == nil {
			n++
		}
	}
	return n, nil
}

type storeReporter struct {
	store       RunStore
	runID       string
	mu          sync.Mutex
	lastWrite   time.Time
	minInterval time.Duration
}

func NewReporter(store RunStore, runID string) Reporter {
	return &storeReporter{store: store, runID: runID, minInterval: 500 * time.Millisecond}
}

func (r *storeReporter) Progress(ctx context.Context, percent int, message string) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if len(message) > 256 {
		message = message[:256]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if percent < 100 && !r.lastWrite.IsZero() && now.Sub(r.lastWrite) < r.minInterval {
		return nil
	}
	r.lastWrite = now
	return r.store.UpdateFields(ctx, r.runID, map[string]any{
		"progress":         percent,
		"progress_message": message,
	})
}

func (r *storeReporter) SetResultRef(ctx context.Context, ref string) error {
	if len(ref) > 512 {
		ref = ref[:512]
	}
	return r.store.UpdateFields(ctx, r.runID, map[string]any{"result_ref": ref})
}
