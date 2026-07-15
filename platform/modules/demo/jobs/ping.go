package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
)

const (
	TypePing           = "demo.ping"
	EventJobPingV1     = "demo.job_ping.v1"
	HandlerEnqueuePing = "demo.enqueue_ping_job"
)

type PingPayload struct {
	Message string `json:"message"`
	Steps   int    `json:"steps"`
}

type JobPingPayload struct {
	Message string `json:"message"`
	Steps   int    `json:"steps"`
}

// Register registers all demo jobs and related outbox handlers.
func Register(a *app.App) error {
	if a.Jobs == nil {
		return nil
	}
	if err := a.Jobs.Register(job.Definition{
		Type: TypePing, SchemaVersion: 1, Queue: job.QueueDefault,
		MaxAttempts: 3, Timeout: 30 * time.Second, Description: "Demo ping job with progress",
	}, job.HandlerFunc{
		JobType: TypePing,
		Fn:      handlePing,
	}); err != nil {
		return err
	}
	if a.Outbox == nil {
		return nil
	}
	if err := a.Outbox.RegisterDefinition(outbox.Definition{
		EventType: EventJobPingV1, SchemaVersion: 1, Description: "Request demo.ping job",
	}); err != nil {
		return err
	}
	return a.Outbox.RegisterHandler(outbox.HandlerFunc{
		HandlerName: HandlerEnqueuePing,
		Types:       []string{EventJobPingV1},
		Fn: func(ctx context.Context, env outbox.Envelope) error {
			if !a.Jobs.Enabled {
				logging.Warn(ctx, "jobs disabled; skip enqueue", "event_id", env.ID, "event_type", env.EventType)
				return nil
			}
			var p JobPingPayload
			_ = json.Unmarshal(env.Payload, &p)
			if p.Steps <= 0 {
				p.Steps = 3
			}
			if p.Message == "" {
				p.Message = "pong"
			}
			_, err := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{
				Type:           TypePing,
				IdempotencyKey: env.ID + ":" + TypePing,
				CausationID:    env.ID,
				Payload:        PingPayload{Message: p.Message, Steps: p.Steps},
			})
			return err
		},
	})
}

func handlePing(ctx context.Context, env job.Envelope, payload json.RawMessage, reporter job.Reporter) error {
	var p PingPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return job.Permanent(err)
	}
	if p.Steps <= 0 {
		p.Steps = 3
	}
	for i := 1; i <= p.Steps; i++ {
		if err := ctx.Err(); err != nil {
			return job.Cancelled(err)
		}
		pct := (i * 100) / p.Steps
		_ = reporter.Progress(ctx, pct, p.Message)
		time.Sleep(20 * time.Millisecond)
	}
	_ = reporter.SetResultRef(ctx, "demo:ping:"+env.ID)
	return nil
}
