package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
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
	}
	wantPaths := map[string]string{
		operations.ActionJobList:                  "/api/v1/operations/jobs/list",
		operations.ActionOutboxList:               "/api/v1/operations/outbox/list",
		operations.ActionNotificationDeliveryList: "/api/v1/operations/notification-deliveries/list",
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
	menus := map[string]struct{ parent, permission string }{}
	for _, item := range application.Menus.List() {
		menus[item.ID] = struct{ parent, permission string }{item.ParentID, item.Permission}
	}
	for id, want := range map[string]struct{ parent, permission string }{
		"operations.jobs":                    {"operations.root", operations.ActionJobList},
		"operations.outbox":                  {"operations.root", operations.ActionOutboxList},
		"operations.notification_deliveries": {"operations.root", operations.ActionNotificationDeliveryList},
	} {
		if menus[id] != want {
			t.Fatalf("menu %s=%#v want=%#v", id, menus[id], want)
		}
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
	result, err := operations.NewOutboxService(db).List(ctx, json.RawMessage(`{"event_type":"demo.a.v1","status":"failed","current":1,"size":10}`))
	if err != nil {
		t.Fatal(err)
	}
	data := result.(map[string]any)
	items := data["items"].([]outbox.Event)
	if data["total"].(int64) != 1 || len(items) != 1 || items[0].ID != "t1-a" {
		t.Fatalf("unexpected tenant-scoped list: %#v", data)
	}
}
