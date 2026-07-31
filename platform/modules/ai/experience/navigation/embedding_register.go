package navigation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
)

const NavigationEmbeddingJobType = "ai.navigation.embed"

type navigationEmbeddingJobPayload struct {
	Documents       []NavigationDocumentRef `json:"documents,omitempty"`
	DocumentID      string                  `json:"document_id,omitempty"`
	CatalogRevision uint64                  `json:"catalog_revision,omitempty"`
	ModelAlias      string                  `json:"model_alias"`
}

func (p navigationEmbeddingJobPayload) documentRefs() []NavigationDocumentRef {
	return NavigationDocumentsChangedPayload{Documents: p.Documents, DocumentID: p.DocumentID, CatalogRevision: p.CatalogRevision}.documentRefs()
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
		if json.Unmarshal(run.PayloadJSON, &payload) != nil || len(payload.documentRefs()) == 0 || payload.ModelAlias == "" {
			return job.Permanent(errors.New("trusted JobRun payload invalid"))
		}
		ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: run.TenantID, UserID: run.CreatedBy, Realm: auth.RealmTenant, RequestID: run.CorrelationID, Client: "job"})
		_ = reporter.Progress(ctx, 10, "navigation.embedding")
		if err = navigation.IndexDocuments(ctx, payload.documentRefs(), payload.ModelAlias); err != nil {
			if errors.Is(err, ErrNavigationEmbeddingSuperseded) {
				_ = reporter.Progress(ctx, 100, "navigation.superseded")
				return nil
			}
			return job.Retryable(errors.New("navigation embedding failed"))
		}
		_ = reporter.Progress(ctx, 100, "navigation.ready")
		return nil
	}}); err != nil {
		return err
	}
	return a.Outbox.RegisterHandler(outbox.HandlerFunc{HandlerName: "ai.enqueue_navigation_embedding", Types: []string{navigationDocumentChangedEvent}, Fn: func(ctx context.Context, env outbox.Envelope) error {
		var changed NavigationDocumentsChangedPayload
		if json.Unmarshal(env.Payload, &changed) != nil || len(changed.documentRefs()) == 0 {
			return errors.New("navigation document event payload invalid")
		}
		ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: env.TenantID, UserID: env.ActorID, Realm: auth.RealmTenant, RequestID: env.CorrelationID, Client: "outbox"})
		alias, err := navigation.embeddingAlias(ctx, env.TenantID)
		if err != nil {
			return err
		}
		if alias == "" {
			return nil
		}
		if !a.Jobs.Enabled {
			err := navigation.IndexDocuments(ctx, changed.documentRefs(), alias)
			if errors.Is(err, ErrNavigationEmbeddingSuperseded) {
				return nil
			}
			return err
		}
		_, err = a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{Type: NavigationEmbeddingJobType, IdempotencyKey: "navigation-batch:" + env.ID, CausationID: env.ID, Payload: navigationEmbeddingJobPayload{Documents: changed.documentRefs(), ModelAlias: alias}})
		return err
	}})
}
