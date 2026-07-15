package job

import (
	"context"
	"encoding/json"
	"time"
)

// Handler executes a job. Must be idempotent and honor ctx cancellation.
type Handler interface {
	Type() string
	Handle(ctx context.Context, env Envelope, payload json.RawMessage, reporter Reporter) error
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc struct {
	JobType string
	Fn      func(ctx context.Context, env Envelope, payload json.RawMessage, reporter Reporter) error
}

func (h HandlerFunc) Type() string { return h.JobType }
func (h HandlerFunc) Handle(ctx context.Context, env Envelope, payload json.RawMessage, reporter Reporter) error {
	return h.Fn(ctx, env, payload, reporter)
}

// Reporter updates progress/result without Handler touching job_runs directly.
type Reporter interface {
	Progress(ctx context.Context, percent int, message string) error
	SetResultRef(ctx context.Context, ref string) error
}

// Transport enqueues minimal messages to the queue backend (Asynq adapter).
type Transport interface {
	Enqueue(ctx context.Context, env Envelope, queue string, opts EnqueueTransportOpts) error
	Ping(ctx context.Context) error
	Close() error
}

// EnqueueTransportOpts are transport-level options expressed via framework types.
type EnqueueTransportOpts struct {
	TaskID    string
	MaxRetry  int
	Timeout   time.Duration
	ProcessIn time.Duration
	UniqueFor time.Duration
	Retention time.Duration
}
