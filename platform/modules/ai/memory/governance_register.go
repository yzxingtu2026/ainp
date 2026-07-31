package memory

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
)

type memoryProjectionPayload struct {
	UserID string `json:"user_id"`
}

func RegisterMemoryGovernance(a *app.App, projection *MemoryProjectionService, consolidation *MemoryConsolidationService) error {
	if a.Outbox != nil {
		if err := a.Outbox.RegisterDefinition(outbox.Definition{EventType: MemoryItemChangedEvent, SchemaVersion: 1, Description: "Memory item lifecycle changed"}); err != nil {
			return err
		}
		if err := a.Outbox.RegisterHandler(outbox.HandlerFunc{HandlerName: "ai.enqueue_memory_projection", Types: []string{MemoryItemChangedEvent}, Fn: func(ctx context.Context, env outbox.Envelope) error {
			var p memoryProjectionPayload
			if json.Unmarshal(env.Payload, &p) != nil || p.UserID == "" {
				return nil
			}
			if a.Jobs == nil || !a.Jobs.Enabled {
				return nil
			}
			ctx = auth.WithContext(ctx, &auth.Context{TenantID: env.TenantID, Realm: auth.NormalizeRealm(auth.Realm(env.Realm)), UserID: env.ActorID, RequestID: env.CorrelationID, Authenticated: true, Client: "outbox"})
			if _, err := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{Type: MemoryProjectionJobType, IdempotencyKey: env.ID + ":" + MemoryProjectionJobType, CausationID: env.ID, Payload: p}); err != nil {
				return err
			}
			_, err := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{Type: MemoryConsolidationJobType, IdempotencyKey: env.ID + ":" + MemoryConsolidationJobType, CausationID: env.ID, ProcessIn: 5 * time.Second, Payload: p})
			return err
		}}); err != nil {
			return err
		}
	}
	if a.Jobs != nil {
		if err := a.Jobs.Register(job.Definition{Type: MemoryProjectionJobType, SchemaVersion: 1, Queue: job.QueueAI, MaxAttempts: 5, Timeout: 2 * time.Minute, Description: "Rebuild memory day and current profile projections"}, job.HandlerFunc{JobType: MemoryProjectionJobType, Fn: func(ctx context.Context, env job.Envelope, payload json.RawMessage, reporter job.Reporter) error {
			var p memoryProjectionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return job.Permanent(err)
			}
			if err := projection.Rebuild(ctx, p.UserID); err != nil {
				return err
			}
			return reporter.SetResultRef(ctx, "ai_memory_profile:"+p.UserID)
		}}); err != nil {
			return err
		}
		return a.Jobs.Register(job.Definition{Type: MemoryConsolidationJobType, SchemaVersion: 1, Queue: job.QueueAI, MaxAttempts: 5, Timeout: 2 * time.Minute, Description: "Create and validate a memory consolidation plan"}, job.HandlerFunc{JobType: MemoryConsolidationJobType, Fn: func(ctx context.Context, env job.Envelope, payload json.RawMessage, reporter job.Reporter) error {
			var p memoryProjectionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return job.Permanent(err)
			}
			if consolidation == nil {
				return nil
			}
			if err := consolidation.Consolidate(ctx, p.UserID); err != nil {
				return err
			}
			return reporter.SetResultRef(ctx, "ai_memory_consolidation:"+p.UserID)
		}})
	}
	return nil
}
