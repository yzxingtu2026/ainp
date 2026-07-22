package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
)

const ActionPersonalizationGet = "ai.assistant_personalization_get"
const ActionPersonalizationRebuild = "ai.assistant_personalization_rebuild"

type personalizationJobPayload struct {
	UserID             string `json:"user_id"`
	SlotKey            string `json:"slot_key"`
	GenerationRevision uint64 `json:"generation_revision"`
}

func RegisterPersonalization(a *app.App, service *PersonalizationService) error {
	if a.Outbox == nil || a.Jobs == nil {
		return errors.New("assistant personalization requires Outbox and Job runtimes")
	}
	if err := a.Outbox.RegisterDefinition(outbox.Definition{EventType: AssistantPersonalizationEvent, SchemaVersion: 1, Description: "Assistant personalization refresh requested"}); err != nil {
		return err
	}
	if err := a.Jobs.Register(job.Definition{Type: AssistantPersonalizationJobType, SchemaVersion: 1, Queue: job.QueueAI, MaxAttempts: 5, Timeout: 2 * time.Minute, UniqueFor: 5 * time.Minute, Description: "Generate one assistant personalization snapshot"}, job.HandlerFunc{JobType: AssistantPersonalizationJobType, Fn: func(ctx context.Context, env job.Envelope, payload json.RawMessage, _ job.Reporter) error {
		var p personalizationJobPayload
		if json.Unmarshal(payload, &p) != nil || p.UserID == "" || p.SlotKey == "" {
			return job.Permanent(errors.New("invalid personalization payload"))
		}
		return service.Generate(ctx, p.UserID, p.SlotKey, p.GenerationRevision)
	}}); err != nil {
		return err
	}
	if err := a.Outbox.RegisterHandler(outbox.HandlerFunc{HandlerName: "ai.enqueue_assistant_personalization", Types: []string{AssistantPersonalizationEvent}, Fn: func(ctx context.Context, env outbox.Envelope) error {
		var p personalizationJobPayload
		if json.Unmarshal(env.Payload, &p) != nil {
			return errors.New("invalid personalization event")
		}
		ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: env.TenantID, UserID: env.ActorID, Realm: auth.RealmTenant, RequestID: env.CorrelationID, Client: "outbox"})
		if !a.Jobs.Enabled {
			return nil
		}
		_, e := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{Type: AssistantPersonalizationJobType, IdempotencyKey: fmt.Sprintf("assistant-personalization:%s:%s:%s:%d", env.TenantID, p.UserID, p.SlotKey, p.GenerationRevision), ProcessIn: time.Minute, Payload: p, CausationID: env.ID})
		return e
	}}); err != nil {
		return err
	}
	if err := a.Outbox.RegisterHandler(outbox.HandlerFunc{HandlerName: "ai.invalidate_assistant_personalization_after_run", Types: []string{agentRunTerminalEvent}, Fn: func(ctx context.Context, env outbox.Envelope) error {
		var payload struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(env.Payload, &payload) != nil || payload.Status != "completed" || env.ActorID == "" {
			return nil
		}
		ctx = auth.WithContext(ctx, &auth.Context{Authenticated: true, TenantID: env.TenantID, UserID: env.ActorID, Realm: auth.RealmTenant, RequestID: env.CorrelationID, Client: "outbox"})
		return service.InvalidateAndRequest(ctx, env.ActorID, uint64(env.AggregateVersion))
	}}); err != nil {
		return err
	}
	auditOff := false
	for _, x := range []struct {
		id, path, name string
		handler        action.Handler
	}{{ActionPersonalizationGet, "/api/v1/ai/assistant-personalization/get", "读取助手个性化快照", action.JSON(func(ctx context.Context, in struct{}) (any, error) { return service.Read(ctx, in) })}, {ActionPersonalizationRebuild, "/api/v1/ai/assistant-personalization/rebuild", "重建助手个性化快照", action.JSON(func(ctx context.Context, in struct{}) (any, error) { return service.Rebuild(ctx, in) })}} {
		if err := a.RegisterAction(action.Action{ID: x.id, Handler: x.handler, Meta: action.Metadata{Capability: "ai", Access: action.AccessAuthenticated, Name: x.name, Description: x.name, Audit: &auditOff, HTTP: &action.HTTPMeta{Method: "POST", Path: x.path}}}); err != nil {
			return err
		}
	}
	return nil
}
