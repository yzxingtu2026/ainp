package catalog

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
)

type Config struct {
	ModuleID       string
	CapabilityID   string
	AgentPromptKey string
}

func Prepare(application *app.App) error {
	if err := application.RegisterAuthenticator(exampleAuthenticator{}); err != nil {
		return err
	}
	if err := application.RegisterPrincipalProvider(customerProvider{}); err != nil {
		return err
	}
	if err := application.RegisterClientSurface(auth.ClientSurface{
		ID: CustomerSurface, Audience: CustomerAudience, PathPrefix: "/api/v1/customer/",
		PrincipalKinds: []auth.PrincipalKind{CustomerKind}, Methods: []string{"example-sso"},
	}); err != nil {
		return err
	}
	return application.RegisterWebSocketHandshakeTarget(auth.WebSocketHandshakeTarget{
		ID: "orders.customer-events", Path: "/api/v1/customer/events",
	})
}

func Register(application *app.App, config Config) error {
	if err := application.PlatformResources.Validate(); err != nil {
		return err
	}
	service := Service{Resources: application.PlatformResources}
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
	for _, definition := range []struct {
		id, name, permission, method, path string
		handler                            action.Handler
	}{
		{"orders.attachment_read", "Read order attachment", "orders:read", "POST", "/api/v1/orders/attachments/read", action.JSON(service.ReadAttachment)},
		{"orders.attachment_binding_exists", "Check order attachment binding", "orders:read", "POST", "/api/v1/orders/attachments/binding/exists", action.JSON(service.AttachmentBindingExists)},
		{"orders.principal_get", "Get account projection", "orders:read", "POST", "/api/v1/orders/principals/get", action.JSON(service.Principal)},
		{"orders.principal_invalidate", "Invalidate account sessions", "orders:manage", "POST", "/api/v1/orders/principals/invalidate", action.JSON(service.InvalidatePrincipal)},
		{"orders.account_identity_list", "List current account identities", "orders:read", "GET", "/api/v1/orders/account-identities", service.ListAccountIdentities},
		{"orders.account_identity_unbind", "Unbind current account identity", "orders:manage", "POST", "/api/v1/orders/account-identities/unbind", action.JSON(service.UnbindAccountIdentity)},
	} {
		if err := application.RegisterAction(action.Action{ID: definition.id, Handler: definition.handler, Meta: action.Metadata{Capability: config.CapabilityID, Access: action.AccessPermission, Name: definition.name, Permission: definition.permission, AI: action.ExcludeFromAI(action.AIExcludePendingSafetyReview), HTTP: &action.HTTPMeta{Method: definition.method, Path: definition.path}}}); err != nil {
			return err
		}
	}
	if err := application.RegisterAction(action.Action{
		ID: "orders.customer_list", Handler: service.List,
		Meta: action.Metadata{Capability: config.CapabilityID, Access: action.AccessPermission, Permission: "orders:read",
			PrincipalKinds: []auth.PrincipalKind{CustomerKind}, ClientSurfaces: []string{CustomerSurface},
			Name: "List customer orders", AI: action.ExcludeFromAI(action.AIExcludePendingSafetyReview),
			HTTP: &action.HTTPMeta{Method: "GET", Path: "/api/v1/customer/orders"}},
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
