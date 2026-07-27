package asynqadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/diagnostic"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"gorm.io/gorm"
)

const taskType = "ainp.job.run"

type message struct {
	JobRunID string `json:"job_run_id"`
}

// RedisOpt builds Asynq Redis client options from config fields.
type RedisOpt struct {
	Addr         string
	Username     string
	Password     string
	DB           int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolSize     int
}

func (o RedisOpt) asynqRedis() asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:         o.Addr,
		Username:     o.Username,
		Password:     o.Password,
		DB:           o.DB,
		DialTimeout:  o.DialTimeout,
		ReadTimeout:  o.ReadTimeout,
		WriteTimeout: o.WriteTimeout,
		PoolSize:     o.PoolSize,
	}
}

// ClientTransport implements job.Transport via Asynq.
type ClientTransport struct {
	client    *asynq.Client
	inspector *asynq.Inspector
}

func NewClientTransport(opt RedisOpt) *ClientTransport {
	redis := opt.asynqRedis()
	return &ClientTransport{
		client:    asynq.NewClient(redis),
		inspector: asynq.NewInspector(redis),
	}
}

func (t *ClientTransport) Enqueue(ctx context.Context, env job.Envelope, queue string, opts job.EnqueueTransportOpts) error {
	payload, err := json.Marshal(message{JobRunID: env.ID})
	if err != nil {
		return err
	}
	task := asynq.NewTask(taskType, payload, asynq.TaskID(opts.TaskID))
	asynqOpts := []asynq.Option{
		asynq.Queue(queue),
		asynq.MaxRetry(opts.MaxRetry),
		asynq.Timeout(opts.Timeout),
	}
	if opts.ProcessIn > 0 {
		asynqOpts = append(asynqOpts, asynq.ProcessIn(opts.ProcessIn))
	}
	if opts.UniqueFor > 0 {
		asynqOpts = append(asynqOpts, asynq.Unique(opts.UniqueFor))
	}
	if opts.Retention > 0 {
		asynqOpts = append(asynqOpts, asynq.Retention(opts.Retention))
	}
	_, err = t.client.EnqueueContext(ctx, task, asynqOpts...)
	if err != nil {
		if strings.Contains(err.Error(), "task ID conflicts") || strings.Contains(err.Error(), "duplicate") {
			return nil // idempotent success
		}
		return err
	}
	return nil
}

func (t *ClientTransport) Ping(ctx context.Context) error {
	if t.inspector == nil {
		return fmt.Errorf("asynq inspector nil")
	}
	_, err := t.inspector.Queues()
	return err
}

func (t *ClientTransport) Close() error {
	if t.inspector != nil {
		_ = t.inspector.Close()
	}
	if t.client != nil {
		return t.client.Close()
	}
	return nil
}

// WorkerConfig configures the Asynq server.
type WorkerConfig struct {
	Concurrency    int
	Queues         map[string]int
	StrictPriority bool
}

// NewServer creates an Asynq server that dispatches to the job registry.
func NewServer(redis RedisOpt, cfg WorkerConfig, db *gorm.DB, registry *job.Registry, store job.RunStore) *asynq.Server {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}
	if len(cfg.Queues) == 0 {
		cfg.Queues = map[string]int{
			job.QueueCritical: 6, job.QueueDefault: 3, job.QueueFile: 2,
			job.QueueNotification: 2, job.QueueAI: 1,
		}
	}
	return asynq.NewServer(redis.asynqRedis(), asynq.Config{
		Concurrency:    cfg.Concurrency,
		Queues:         cfg.Queues,
		StrictPriority: cfg.StrictPriority,
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			logging.Warn(ctx, "asynq task error", "type", task.Type(), "error", job.TruncateError(err))
		}),
	})
}

// NewMux registers the single ainp.job.run handler.
func NewMux(db *gorm.DB, registry *job.Registry, store job.RunStore) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(taskType, func(ctx context.Context, task *asynq.Task) error {
		return process(ctx, db, registry, store, task)
	})
	return mux
}

func process(ctx context.Context, db *gorm.DB, registry *job.Registry, store job.RunStore, task *asynq.Task) error {
	var msg message
	if err := json.Unmarshal(task.Payload(), &msg); err != nil || msg.JobRunID == "" {
		return fmt.Errorf("invalid job payload: %w", err)
	}
	run, err := job.GetTrusted(ctx, db, msg.JobRunID)
	if err != nil {
		return err
	}
	handler, ok := registry.Handler(run.JobType)
	if !ok {
		_ = store.UpdateFields(ctx, run.ID, map[string]any{
			"status":      status.JobDead,
			"last_error":  "handler not registered",
			"finished_at": time.Now().UTC(),
		})
		return fmt.Errorf("handler missing for %s", run.JobType)
	}
	def, _ := registry.Definition(run.JobType)

	if run.CancelRequestedAt != nil {
		now := time.Now().UTC()
		_ = store.UpdateFields(ctx, run.ID, map[string]any{
			"status": status.JobCancelled, "finished_at": now,
		})
		return nil
	}

	attempt, _ := asynq.GetRetryCount(ctx)
	attempt = attempt + 1
	now := time.Now().UTC()
	queueLatency := now.Sub(run.UpdatedAt).Milliseconds()
	_ = store.UpdateFields(ctx, run.ID, map[string]any{
		"status":     status.JobRunning,
		"attempt":    attempt,
		"started_at": now,
		"last_error": "",
	})

	jobCtx := auth.WithContext(ctx, &auth.Context{
		TenantID:      run.TenantID,
		Realm:         auth.Realm(run.Realm),
		UserID:        run.CreatedBy,
		RequestID:     uuid.NewString(),
		TraceID:       run.CorrelationID,
		Client:        "job",
		Authenticated: true,
	})
	logging.Info(jobCtx, "job started", "job_type", run.JobType, "queue", run.Queue, "job_queue_latency_ms", queueLatency)
	env := job.Envelope{
		ID: run.ID, Type: run.JobType, SchemaVersion: run.SchemaVersion,
		TenantID: run.TenantID, Realm: auth.Realm(run.Realm),
		ActorID: run.CreatedBy, CorrelationID: run.CorrelationID, CausationID: run.CausationID,
		CreatedAt: run.CreatedAt, Attempt: attempt,
	}
	reporter := job.NewReporter(store, run.ID)

	start := time.Now()
	err = handler.Handle(jobCtx, env, json.RawMessage(run.PayloadJSON), reporter)
	dur := time.Since(start)

	// re-check cancel
	latest, _ := job.GetTrusted(ctx, db, run.ID)
	if latest != nil && latest.CancelRequestedAt != nil && err == nil {
		err = job.Cancelled(nil)
	}

	finished := time.Now().UTC()
	if err == nil {
		_ = store.UpdateFields(ctx, run.ID, map[string]any{
			"status": status.JobSucceeded, "progress": 100, "finished_at": finished, "last_error": "",
		})
		logging.Info(jobCtx, "job succeeded",
			"job_id", run.ID, "job_type", run.JobType, "attempt", attempt, "duration_ms", dur.Milliseconds())
		return nil
	}
	if job.IsCancelled(err) {
		_ = store.UpdateFields(ctx, run.ID, map[string]any{
			"status": status.JobCancelled, "finished_at": finished, "last_error": job.TruncateError(err),
		})
		return nil
	}
	if job.IsPermanent(err) {
		_ = store.UpdateFields(ctx, run.ID, map[string]any{
			"status": status.JobDead, "finished_at": finished, "last_error": job.TruncateError(err),
		})
		logJobFailure(jobCtx, "job permanent failure", run.ID, run.JobType, attempt, dur, err, true)
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}

	maxAttempts := run.MaxAttempts
	if def.MaxAttempts > 0 {
		maxAttempts = def.MaxAttempts
	}
	if attempt >= maxAttempts {
		_ = store.UpdateFields(ctx, run.ID, map[string]any{
			"status": status.JobDead, "finished_at": finished, "last_error": job.TruncateError(err),
		})
		logJobFailure(jobCtx, "job dead", run.ID, run.JobType, attempt, dur, err, true)
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	_ = store.UpdateFields(ctx, run.ID, map[string]any{
		"status": status.JobRetrying, "last_error": job.TruncateError(err),
	})
	logging.Warn(jobCtx, "job retryable failure",
		"job_id", run.ID, "job_type", run.JobType, "attempt", attempt, "error", job.TruncateError(err))
	return err
}

func logJobFailure(ctx context.Context, message, id, jobType string, attempt int, duration time.Duration, err error, final bool) {
	meta := diagnostic.Describe(err, "job.handler")
	attrs := []any{"job_id", id, "job_type", jobType, "attempt", attempt, "duration_ms", duration.Milliseconds(), "final", final, "error_category", meta.Category, "error_stage", meta.Stage, "safe_summary", meta.SafeSummary}
	if final {
		logging.Error(ctx, message, err, attrs...)
		return
	}
	logging.Warn(ctx, message, append(attrs, "error", job.TruncateError(err))...)
}
