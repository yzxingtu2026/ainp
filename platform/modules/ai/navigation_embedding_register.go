package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
)

const NavigationEmbeddingJobType = "ai.navigation.embed"

type navigationEmbeddingJobPayload struct {
	TenantID string `json:"tenant_id"`
}

func RegisterNavigationEmbedding(a *app.App, navigation *NavigationSearchService) error {
	if a.Jobs == nil || a.Outbox == nil {
		return errors.New("navigation embedding requires Job runtime")
	}
	if err := a.Jobs.Register(job.Definition{Type: NavigationEmbeddingJobType, SchemaVersion: 1, Queue: job.QueueAI, MaxAttempts: 5, Timeout: 5 * time.Minute, UniqueFor: time.Hour, Description: "Build governed pgvector projections for navigation documents"}, job.HandlerFunc{JobType: NavigationEmbeddingJobType, Fn: func(ctx context.Context, env job.Envelope, _ json.RawMessage, reporter job.Reporter) error {
		run, err := job.GetTrusted(ctx, a.DB, env.ID)
		if err != nil {
			return job.Permanent(errors.New("trusted JobRun unavailable"))
		}
		var payload navigationEmbeddingJobPayload
		if json.Unmarshal(run.PayloadJSON, &payload) != nil || payload.TenantID == "" || payload.TenantID != run.TenantID {
			return job.Permanent(errors.New("trusted JobRun payload invalid"))
		}
		ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: run.TenantID, UserID: run.CreatedBy, Realm: auth.RealmTenant, RequestID: run.CorrelationID, Client: "job"})
		_ = reporter.Progress(ctx, 10, "navigation.embedding")
		if err = navigation.IndexPending(ctx, run.TenantID); err != nil {
			return job.Retryable(errors.New("navigation embedding failed"))
		}
		_ = reporter.Progress(ctx, 100, "navigation.ready")
		return nil
	}}); err != nil {
		return err
	}
	return a.Outbox.RegisterHandler(outbox.HandlerFunc{HandlerName: "ai.enqueue_navigation_embedding", Types: []string{navigationDocumentChangedEvent}, Fn: func(ctx context.Context, env outbox.Envelope) error {
		var changed NavigationDocumentChangedPayload
		if json.Unmarshal(env.Payload, &changed) != nil || changed.DocumentID == "" || changed.CatalogRevision == 0 {
			return errors.New("navigation document event payload invalid")
		}
		ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: env.TenantID, UserID: env.ActorID, Realm: auth.RealmTenant, RequestID: env.CorrelationID, Client: "outbox"})
		if !a.Jobs.Enabled {
			return navigation.IndexPending(ctx, env.TenantID)
		}
		_, err := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{Type: NavigationEmbeddingJobType, IdempotencyKey: "navigation:" + changed.DocumentID + ":" + fmt.Sprint(changed.CatalogRevision), CausationID: env.ID, Payload: navigationEmbeddingJobPayload{TenantID: env.TenantID}})
		return err
	}})
}
