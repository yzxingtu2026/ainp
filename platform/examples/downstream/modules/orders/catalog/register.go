package catalog

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
)

type Config struct {
	ModuleID       string
	CapabilityID   string
	AgentPromptKey string
}

func Prepare(application *app.App) error {
	return application.Authenticators.Register(exampleAuthenticator{})
}

func Register(application *app.App, config Config) error {
	service := Service{}
	if err := application.RegisterCapability(capability.Capability{ID: config.CapabilityID, Name: "Orders", Module: config.ModuleID}); err != nil {
		return err
	}
	if err := application.RegisterAction(action.Action{
		ID:      "orders.list",
		Handler: service.List,
		Meta: action.Metadata{
			Capability: config.CapabilityID,
			Access:     action.AccessPermission,
			Name:       "List orders",
			Permission: "orders:read",
			AI:         action.ExcludeFromAI(action.AIExcludePendingSafetyReview),
			HTTP:       &action.HTTPMeta{Method: "GET", Path: "/api/v1/orders"},
		},
	}); err != nil {
		return err
	}
	if err := application.RegisterAction(action.Action{
		ID:      "orders.batch_create",
		Handler: action.JSON(service.BatchCreate),
		Meta: action.Metadata{
			Capability: config.CapabilityID,
			Access:     action.AccessPermission,
			Name:       "Create orders",
			Permission: "orders:create",
			AI: &action.AIMeta{
				ExposeTool: true, Operation: action.AIOperationCreate,
				Batch:     &action.AIBatchMeta{Mode: action.AIBatchAtomic, ItemsPath: "items"},
				ToolTitle: "Create orders", Summary: "Create one or more orders atomically", HumanDescription: "Create one or more orders in request order", ResultSummary: "Orders created",
				Risk: "high", SideEffect: "write", RequiresConfirmation: true,
				InputSchema: action.InputSchemaFor[orderBatchCreateRequest](), OutputSchema: action.SchemaFor[orderBatchCreateResponse](),
			},
			Idempotency: &action.IdempotencyMeta{Mode: "required", TTL: 24 * time.Hour},
			HTTP:        &action.HTTPMeta{Method: "POST", Path: "/api/v1/orders/batch/create"},
		},
	}); err != nil {
		return err
	}
	return application.Jobs.Register(
		job.Definition{Type: "orders.sync", SchemaVersion: 1, Queue: job.QueueDefault, MaxAttempts: 3, Timeout: time.Minute, Description: "Sync downstream orders"},
		job.HandlerFunc{JobType: "orders.sync", Fn: func(ctx context.Context, envelope job.Envelope, payload json.RawMessage, reporter job.Reporter) error {
			return service.Sync(ctx, envelope, payload, reporter)
		}},
	)
}
