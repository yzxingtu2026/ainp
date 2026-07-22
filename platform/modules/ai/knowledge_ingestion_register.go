package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"gorm.io/gorm"
)

const ActionKnowledgeDocumentRetry = "ai.knowledge_document_ingestion_retry"

type KnowledgeIngestionRetryRequest struct {
	ID string `json:"id" validate:"required"`
}

func RegisterKnowledgeIngestion(a *app.App, knowledge *KnowledgeService, ingestion *KnowledgeIngestionService) error {
	if a.Outbox == nil || a.Jobs == nil {
		return errors.New("knowledge ingestion requires Outbox and Job runtimes")
	}
	for _, definition := range []outbox.Definition{
		{EventType: KnowledgeIngestionRequestedEvent, SchemaVersion: 1, Description: "Request knowledge document ingestion"},
		{EventType: KnowledgeCleanupRequestedEvent, SchemaVersion: 1, Description: "Request derived knowledge index cleanup"},
	} {
		if err := a.Outbox.RegisterDefinition(definition); err != nil {
			return err
		}
	}
	knowledge.SetEventWriter(a.Outbox.Writer)
	if err := a.Jobs.Register(job.Definition{Type: KnowledgeIngestionJobType, SchemaVersion: 1, Queue: job.QueueAI, MaxAttempts: 3, Timeout: 15 * time.Minute, UniqueFor: time.Hour, Description: "Parse, embed and atomically publish a knowledge document index"}, job.HandlerFunc{JobType: KnowledgeIngestionJobType, Fn: func(ctx context.Context, env job.Envelope, _ json.RawMessage, reporter job.Reporter) error {
		run, err := job.GetTrusted(ctx, a.DB, env.ID)
		if err != nil {
			return job.Permanent(errors.New("trusted JobRun unavailable"))
		}
		if run.JobType != KnowledgeIngestionJobType || run.TenantID == "" || run.SchemaVersion != 1 {
			return job.Permanent(errors.New("trusted JobRun contract mismatch"))
		}
		if run.CancelRequestedAt != nil {
			return job.Cancelled(errors.New("knowledge ingestion cancelled"))
		}
		var payload KnowledgeIngestionJobPayload
		if err = json.Unmarshal(run.PayloadJSON, &payload); err != nil {
			return job.Permanent(errors.New("trusted JobRun payload invalid"))
		}
		ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: run.TenantID, UserID: run.CreatedBy, Realm: auth.Realm(run.Realm), RequestID: run.CorrelationID, Client: "job"})
		_ = reporter.Progress(ctx, 10, "knowledge_ingestion.processing")
		if payload.Operation == "cleanup" {
			err = ingestion.Cleanup(ctx, payload.DocumentID)
		} else {
			err = ingestion.Process(ctx, payload)
		}
		if err != nil {
			return err
		}
		_ = reporter.Progress(ctx, 100, "knowledge_ingestion.completed")
		_ = reporter.SetResultRef(ctx, "ai_knowledge_document:"+payload.DocumentID)
		return nil
	}}); err != nil {
		return err
	}
	if err := a.Outbox.RegisterHandler(outbox.HandlerFunc{HandlerName: knowledgeIngestionHandler, Types: []string{KnowledgeIngestionRequestedEvent, KnowledgeCleanupRequestedEvent}, Fn: func(ctx context.Context, env outbox.Envelope) error {
		if !a.Jobs.Enabled {
			return job.ErrUnavailable
		}
		var payload KnowledgeIngestionJobPayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return err
		}
		if env.EventType == KnowledgeCleanupRequestedEvent {
			payload.Operation = "cleanup"
		} else {
			payload.Operation = "ingest"
		}
		ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: env.TenantID, UserID: env.ActorID, Realm: auth.Realm(env.Realm), RequestID: env.CorrelationID, Client: "outbox"})
		_, err := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{Type: KnowledgeIngestionJobType, IdempotencyKey: env.ID + ":" + KnowledgeIngestionJobType, CausationID: env.ID, Payload: payload})
		return err
	}}); err != nil {
		return err
	}
	auditOn := true
	return a.RegisterAction(action.Action{ID: ActionKnowledgeDocumentRetry, Handler: action.JSON(func(ctx context.Context, request KnowledgeIngestionRetryRequest) (any, error) {
		document, _, _, err := knowledge.getDocument(ctx, request.ID, true)
		if err != nil {
			return nil, err
		}
		err = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return appendKnowledgeIngestion(ctx, tx, a.Outbox.Writer, *document) })
		if err != nil {
			return nil, err
		}
		return map[string]any{"requested": true, "document_id": document.ID, "revision": document.Revision}, nil
	}), Meta: action.Metadata{Capability: "ai", Name: "Retry knowledge ingestion", NameKey: "action." + ActionKnowledgeDocumentRetry + ".name", Description: "Request a new reliable ingestion attempt", DescriptionKey: "action." + ActionKnowledgeDocumentRetry + ".description", PermissionGroup: "ai.knowledge_management", PermissionGroupName: "Knowledge management", PermissionGroupNameKey: "permission_group.ai.knowledge_management", Audit: &auditOn, AI: action.ExcludeFromAI(action.AIExcludeInternalRuntime), HTTP: &action.HTTPMeta{Method: "POST", Path: "/api/v1/ai/knowledge-documents/ingestion/retry"}}})
}
