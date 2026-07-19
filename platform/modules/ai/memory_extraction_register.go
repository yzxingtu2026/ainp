package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
)

type memoryExtractionJobPayload struct {
	RunID string `json:"run_id"`
}
type permanentExtractionError struct{ err error }

func (e permanentExtractionError) Error() string { return e.err.Error() }
func (e permanentExtractionError) Unwrap() error { return e.err }
func jobPermanent(err error) error               { return permanentExtractionError{err: err} }

func RegisterMemoryExtraction(a *app.App, service *MemoryExtractionService, governance *MemoryGovernanceService) error {
	if a.Outbox != nil {
		if err := a.Outbox.RegisterHandler(outbox.HandlerFunc{HandlerName: "ai.enqueue_memory_extraction", Types: []string{agentRunTerminalEvent}, Fn: func(ctx context.Context, env outbox.Envelope) error {
			var payload struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(env.Payload, &payload) != nil || payload.Status != "completed" {
				return nil
			}
			if a.Jobs == nil || !a.Jobs.Enabled {
				logging.Warn(ctx, "jobs disabled; memory evaluation pending", "run_id", env.AggregateID)
				return nil
			}
			ctx = auth.WithContext(ctx, &auth.Context{TenantID: env.TenantID, Realm: auth.NormalizeRealm(auth.Realm(env.Realm)), UserID: env.ActorID, RequestID: env.CorrelationID, Authenticated: true, Client: "outbox"})
			_, err := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{Type: MemoryExtractionJobType, IdempotencyKey: env.ID + ":" + MemoryExtractionJobType, CausationID: env.ID, Payload: memoryExtractionJobPayload{RunID: env.AggregateID}})
			return err
		}}); err != nil {
			return err
		}
	}
	if a.Jobs != nil {
		return a.Jobs.Register(job.Definition{Type: MemoryExtractionJobType, SchemaVersion: 1, Queue: job.QueueAI, MaxAttempts: 5, Timeout: 2 * time.Minute, Description: "Evaluate one completed Agent Run for memory candidates"}, job.HandlerFunc{JobType: MemoryExtractionJobType, Fn: func(ctx context.Context, env job.Envelope, payload json.RawMessage, reporter job.Reporter) error {
			var p memoryExtractionJobPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return job.Permanent(err)
			}
			err := service.EvaluateRun(ctx, p.RunID)
			if err == nil && governance != nil {
				err = governance.GovernRun(ctx, p.RunID)
			}
			var permanent permanentExtractionError
			if errors.As(err, &permanent) {
				return job.Permanent(err)
			}
			var runtimeErr *aimodelruntime.RuntimeError
			if errors.As(err, &runtimeErr) && !runtimeErr.Retryable {
				return job.Permanent(err)
			}
			if err == nil {
				_ = reporter.SetResultRef(ctx, "ai_memory_evaluation:"+p.RunID)
			}
			return err
		}})
	}
	return nil
}
