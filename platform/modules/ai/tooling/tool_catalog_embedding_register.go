package tooling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
)

const ToolCatalogEmbeddingJobType = "ai.tool_catalog.embed"

type toolCatalogEmbeddingJobPayload struct {
	ToolID          string `json:"tool_id"`
	CatalogRevision uint64 `json:"catalog_revision"`
	ModelAlias      string `json:"model_alias"`
}

func RegisterToolCatalogEmbedding(a *app.App, tools *ToolService) error {
	if a.Outbox == nil || a.Jobs == nil {
		return errors.New("tool catalog embedding requires Outbox and Job runtimes")
	}
	if err := a.Jobs.Register(job.Definition{Type: ToolCatalogEmbeddingJobType, SchemaVersion: 1, Queue: job.QueueAI, MaxAttempts: 5, Timeout: 2 * time.Minute, UniqueFor: time.Hour, Description: "Build a governed pgvector projection for one tool catalog revision"}, job.HandlerFunc{JobType: ToolCatalogEmbeddingJobType, Fn: func(ctx context.Context, env job.Envelope, _ json.RawMessage, reporter job.Reporter) error {
		run, err := job.GetTrusted(ctx, a.DB, env.ID)
		if err != nil {
			return job.Permanent(errors.New("trusted JobRun unavailable"))
		}
		if run.JobType != ToolCatalogEmbeddingJobType || run.TenantID == "" || run.SchemaVersion != 1 {
			return job.Permanent(errors.New("trusted JobRun contract mismatch"))
		}
		var payload toolCatalogEmbeddingJobPayload
		if err = json.Unmarshal(run.PayloadJSON, &payload); err != nil || payload.ToolID == "" || payload.CatalogRevision == 0 || payload.ModelAlias == "" {
			return job.Permanent(errors.New("trusted JobRun payload invalid"))
		}
		ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: run.TenantID, PrincipalID: run.CreatedBy, Realm: auth.Realm(run.Realm), RequestID: run.CorrelationID, Client: "job"})
		_ = reporter.Progress(ctx, 10, "tool_catalog.embedding")
		if err = tools.IndexCatalogDocument(ctx, payload.ToolID, payload.CatalogRevision, payload.ModelAlias); err != nil {
			if errors.Is(err, ErrToolEmbeddingSuperseded) {
				_ = reporter.Progress(ctx, 100, "tool_catalog.superseded")
				return nil
			}
			var runtimeErr *aimodelruntime.RuntimeError
			if errors.As(err, &runtimeErr) && !runtimeErr.Retryable {
				return job.Permanent(errors.New("tool embedding runtime failed"))
			}
			return job.Retryable(errors.New("tool embedding runtime failed"))
		}
		_ = reporter.Progress(ctx, 100, "tool_catalog.ready")
		_ = reporter.SetResultRef(ctx, "ai_tool_catalog:"+payload.ToolID)
		return nil
	}}); err != nil {
		return err
	}
	return a.Outbox.RegisterHandler(outbox.HandlerFunc{HandlerName: "ai.enqueue_tool_catalog_embedding", Types: []string{toolCatalogChangedEvent}, Fn: func(ctx context.Context, env outbox.Envelope) error {
		var changed ToolCatalogChangedPayload
		if err := json.Unmarshal(env.Payload, &changed); err != nil {
			return err
		}
		ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: env.TenantID, PrincipalID: env.ActorID, Realm: auth.Realm(env.Realm), RequestID: env.CorrelationID, Client: "outbox"})
		if changed.Status != "enabled" {
			return nil
		}
		alias, err := tools.embeddingAlias(ctx)
		if err != nil {
			return err
		}
		if alias == "" || !a.Jobs.Enabled {
			return tools.MarkCatalogDocumentReady(ctx, changed)
		}
		payload := toolCatalogEmbeddingJobPayload{ToolID: changed.ToolID, CatalogRevision: changed.CatalogRevision, ModelAlias: alias}
		_, err = a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{Type: ToolCatalogEmbeddingJobType, IdempotencyKey: "tool-catalog:" + changed.ToolID + ":" + fmt.Sprint(changed.CatalogRevision), CausationID: env.ID, Payload: payload})
		return err
	}})
}
