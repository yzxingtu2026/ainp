package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/operations"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestOperationsMetadataKeepsPermissionsAndSplitsGroups(t *testing.T) {
	application := app.New(&config.Config{}, openOutboxTestDB(t), nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	if err := operations.Register(application, system.NewOperationsNotificationDeliveryService(application)); err != nil {
		t.Fatal(err)
	}
	wantGroups := map[string]string{
		operations.ActionJobList:                  "operations.jobs",
		operations.ActionOutboxList:               "operations.outbox",
		system.ActionNotificationTemplateList:     "system.notification_configuration",
		operations.ActionNotificationDeliveryList: "operations.notification_delivery",
		operations.ActionAuditLogList:             "system.operations.audit",
	}
	wantPaths := map[string]string{
		operations.ActionJobList:                  "/api/v1/operations/jobs/list",
		operations.ActionOutboxList:               "/api/v1/operations/outbox/list",
		operations.ActionNotificationDeliveryList: "/api/v1/operations/notification-deliveries/list",
		operations.ActionAuditLogList:             "/api/v1/operations/audit-logs/list",
	}
	for id, group := range wantGroups {
		registered, ok := application.Actions.Get(id)
		if !ok {
			t.Fatalf("action %s not registered", id)
		}
		if registered.Meta.Permission != "" && registered.Meta.Permission != id {
			t.Fatalf("action %s permission changed to %s", id, registered.Meta.Permission)
		}
		if registered.Meta.PermissionGroup != group {
			t.Fatalf("action %s group=%s want=%s", id, registered.Meta.PermissionGroup, group)
		}
		if id != system.ActionNotificationTemplateList {
			if registered.Meta.Capability != "operations" {
				t.Fatalf("action %s capability=%s", id, registered.Meta.Capability)
			}
			if registered.Meta.HTTP == nil || registered.Meta.HTTP.Path != wantPaths[id] {
				t.Fatalf("action %s path=%v want=%s", id, registered.Meta.HTTP, wantPaths[id])
			}
		}
	}
	for _, oldID := range []string{"system.job_list", "system.outbox_list", "system.notification_delivery_list"} {
		if _, ok := application.Actions.Get(oldID); ok {
			t.Fatalf("legacy action must not be registered: %s", oldID)
		}
	}
	menus := map[string]struct{ parent, permission, icon string }{}
	for _, item := range application.Menus.List() {
		menus[item.ID] = struct{ parent, permission, icon string }{item.ParentID, item.Permission, item.Icon}
	}
	for id, want := range map[string]struct{ parent, permission, icon string }{
		"operations.jobs":                    {"operations.root", operations.ActionJobList, "mdi:briefcase-clock-outline"},
		"operations.outbox":                  {"operations.root", operations.ActionOutboxList, "mdi:tray-arrow-up"},
		"operations.notification_deliveries": {"operations.root", operations.ActionNotificationDeliveryList, "mdi:truck-delivery-outline"},
		"operations.audit_logs":              {"operations.root", operations.ActionAuditLogList, "mdi:text-box-search-outline"},
	} {
		if menus[id] != want {
			t.Fatalf("menu %s=%#v want=%#v", id, menus[id], want)
		}
	}
	for _, actionID := range []string{"system.log_source_list", "system.log_query", "system.log_get"} {
		if _, ok := application.Actions.Get(actionID); ok {
			t.Fatalf("log center action must not be registered: %s", actionID)
		}
	}
	if _, ok := menus["operations.logs"]; ok {
		t.Fatal("log center menu must not be registered")
	}

	readActions := []string{
		operations.ActionJobList, operations.ActionJobGet,
		operations.ActionOutboxList, operations.ActionOutboxSummary, operations.ActionOutboxListDeliveries,
		operations.ActionNotificationDeliveryList, operations.ActionNotificationDeliveryGet,
		operations.ActionAuditLogList, operations.ActionAuditLogGet,
	}
	mutationActions := []string{
		operations.ActionJobCancel, operations.ActionJobRetry,
		operations.ActionOutboxRetry, operations.ActionNotificationDeliveryRetry,
	}
	for _, actionID := range readActions {
		assertOperationsAIContract(t, application, actionID, false)
	}
	for _, actionID := range mutationActions {
		assertOperationsAIContract(t, application, actionID, true)
	}
	for _, actionID := range mutationActions {
		registered, _ := application.Actions.Get(actionID)
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(registered.Meta.AI.InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		if _, leaked := schema.Properties["payload"]; leaked {
			t.Fatalf("operations mutation %s must not accept payload", actionID)
		}
	}
	projected, err := (&aitool.ActionProvider{Actions: application.Actions}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	projectedActions := make(map[string]bool, len(projected))
	for _, definition := range projected {
		projectedActions[definition.RuntimeRef] = true
	}
	for _, actionID := range append(readActions, mutationActions...) {
		if !projectedActions[actionID] {
			t.Fatalf("operations Action was not projected from Action Registry: %s", actionID)
		}
	}
}

func assertOperationsAIContract(t *testing.T, application *app.App, actionID string, mutation bool) {
	t.Helper()
	registered, ok := application.Actions.Get(actionID)
	if !ok || registered.Meta.AI == nil || !registered.Meta.AI.ExposeTool {
		t.Fatalf("operations Action %s must expose an AI contract: registered=%t meta=%#v", actionID, ok, registered.Meta.AI)
	}
	meta := registered.Meta.AI
	if !json.Valid(meta.InputSchema) || !json.Valid(meta.OutputSchema) || meta.Summary == "" || meta.HumanDescription == "" || meta.ResultSummary == "" {
		t.Fatalf("operations Action %s has an incomplete AI contract: %#v", actionID, meta)
	}
	if mutation {
		if !meta.RequiresConfirmation || (meta.SideEffect != "write" && meta.SideEffect != "external") || (meta.Risk != "medium" && meta.Risk != "high") {
			t.Fatalf("operations mutation %s has unsafe AI metadata: %#v", actionID, meta)
		}
	} else if meta.RequiresConfirmation || meta.SideEffect != "read" || meta.Risk != "low" {
		t.Fatalf("operations read Action %s has unexpected AI metadata: %#v", actionID, meta)
	}
}

func TestOutboxListFiltersAndKeepsTenantScope(t *testing.T) {
	db := openOutboxTestDB(t)
	now := time.Now().UTC()
	for _, event := range []outbox.Event{
		{ID: "t1-a", TenantID: "t1", Realm: "tenant", EventType: "demo.a.v1", SchemaVersion: 1, PayloadJSON: []byte("{}"), Status: status.OutboxFailed, AvailableAt: now, OccurredAt: now},
		{ID: "t1-b", TenantID: "t1", Realm: "tenant", EventType: "demo.b.v1", SchemaVersion: 1, PayloadJSON: []byte("{}"), Status: status.OutboxPublished, AvailableAt: now, OccurredAt: now},
		{ID: "t2-a", TenantID: "t2", Realm: "tenant", EventType: "demo.a.v1", SchemaVersion: 1, PayloadJSON: []byte("{}"), Status: status.OutboxFailed, AvailableAt: now, OccurredAt: now},
	} {
		if err := db.Create(&event).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant})
	result, err := operations.NewOutboxService(db).List(ctx, json.RawMessage(`{"event_type":"demo.a.v1","status":"failed","page":1,"page_size":10}`))
	if err != nil {
		t.Fatal(err)
	}
	data := result.(pagination.Result[outbox.Event])
	if data.Total != 1 || len(data.Items) != 1 || data.Items[0].ID != "t1-a" {
		t.Fatalf("unexpected tenant-scoped list: %#v", data)
	}
	if _, err := operations.NewOutboxService(db).List(ctx, json.RawMessage(`{"status":"invalid"}`)); errorCode(err) != errs.CodeValidation {
		t.Fatalf("invalid status must fail validation: %v", err)
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0]["id"] != "t1-a" || payload.Items[0]["event_type"] != "demo.a.v1" || payload.Items[0]["status"] != "failed" {
		t.Fatalf("unexpected outbox JSON contract: %s", encoded)
	}
	if _, exposed := payload.Items[0]["TenantID"]; exposed {
		t.Fatalf("outbox JSON must use explicit public fields: %s", encoded)
	}
	ctxB := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t2", Realm: auth.RealmTenant})
	if err := outbox.RetryTenantEvent(ctxB, db, "t1-a"); err == nil {
		t.Fatal("cross-tenant retry must fail closed")
	}
	if err := outbox.RetryTenantEvent(ctx, db, "t1-a"); err != nil {
		t.Fatal(err)
	}
	retried, err := operations.NewOutboxService(db).List(ctx, json.RawMessage(`{"status":"pending","page":1,"page_size":10}`))
	if err != nil {
		t.Fatal(err)
	}
	retriedData := retried.(pagination.Result[outbox.Event])
	if retriedData.Total != 1 || len(retriedData.Items) != 1 || retriedData.Items[0].ID != "t1-a" {
		t.Fatalf("unexpected retried tenant event: %#v", retriedData)
	}
}

func TestNotificationDeliveryListDefaultsToAllStatuses(t *testing.T) {
	db := openOutboxTestDB(t)
	repo := system.NewNotificationRepository(db)
	if err := repo.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant})
	platformCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true})
	systemDB, err := tenant.SystemDB(platformCtx, db)
	if err != nil {
		t.Fatal(err)
	}
	deliveryRepo, err := tenant.NewSystemRepository[system.NotificationDelivery](systemDB)
	if err != nil {
		t.Fatal(err)
	}
	for _, delivery := range []system.NotificationDelivery{
		{ID: "t1-sent", TenantID: "t1", RequestID: "r1", RecipientID: "u1", Channel: "in_app", Status: "sent"},
		{ID: "t1-failed", TenantID: "t1", RequestID: "r2", RecipientID: "u1", Channel: "in_app", Status: "failed"},
		{ID: "t2-sent", TenantID: "t2", RequestID: "r3", RecipientID: "u2", Channel: "in_app", Status: "sent"},
	} {
		if err := deliveryRepo.Create(&delivery); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := repo.ListDeliveries(ctx, 1, 20, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("default delivery list must include all tenant statuses: total=%d items=%#v", total, items)
	}
}
