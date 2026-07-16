package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/modules/operations"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

func TestAuditLogActionsFilterAndEnforceTenantScope(t *testing.T) {
	db := openOutboxTestDB(t)
	if err := db.AutoMigrate(&audit.Record{}, &model.User{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{ID: "u1", TenantID: "t1", Username: "old-user", DisplayName: "Historical User", PasswordHash: "x"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, row := range []audit.Record{
		{ID: "a1", TenantID: "t1", Realm: "tenant", UserID: "u1", ActorType: "user", ActionID: "demo.doc_create", Result: "success", ResourceType: "doc", ResourceID: "d1", RequestID: "req-1", Client: "web", Summary: "created", OccurredAt: now.Add(-time.Hour)},
		{ID: "a2", TenantID: "t1", Realm: "tenant", UserID: "deleted-user", ActorType: "user", ActionID: "demo.doc_update", Result: "failure", ResourceType: "doc", ResourceID: "d2", RequestID: "req-2", Client: "api", OccurredAt: now.Add(-2 * time.Hour)},
		{ID: "a3", TenantID: "t2", Realm: "tenant", UserID: "u2", ActorType: "user", ActionID: "demo.doc_create", Result: "success", ResourceType: "doc", ResourceID: "d3", RequestID: "req-3", Client: "web", OccurredAt: now.Add(-time.Hour)},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	recorder := audit.NewGormRecorder(db)
	application := app.New(&config.Config{}, db, recorder, nil)
	if err := operations.Register(application, nil); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant, UserID: "viewer", Permissions: []string{operations.ActionAuditLogList, operations.ActionAuditLogGet}, RequestID: "query-1", Client: "web"})
	denied := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant})
	if _, err := application.Actions.Invoke(denied, operations.ActionAuditLogList, json.RawMessage(`{}`)); errorCode(err) != errs.CodeForbidden {
		t.Fatalf("audit list without permission must be forbidden: %v", err)
	}
	result, err := application.Actions.Invoke(ctx, operations.ActionAuditLogList, json.RawMessage(`{"user_keyword":"HISTOR","action_id":"demo.doc_create","result":"success","resource_type":"doc","resource_id":"d1","request_id":"req-1","client":"web","page":1,"page_size":20}`))
	if err != nil {
		t.Fatal(err)
	}
	data := result.(pagination.Result[operations.AuditLogItem])
	if data.Total != 1 || len(data.Items) != 1 || data.Items[0].ID != "a1" || data.Items[0].ActorName != "Historical User" {
		t.Fatalf("unexpected audit list: %#v", data)
	}
	usernameResult, err := application.Actions.Invoke(ctx, operations.ActionAuditLogList, json.RawMessage(`{"user_keyword":"OLD-US"}`))
	if err != nil {
		t.Fatal(err)
	}
	usernameData := usernameResult.(pagination.Result[operations.AuditLogItem])
	if usernameData.Total != 1 || len(usernameData.Items) != 1 || usernameData.Items[0].ID != "a1" {
		t.Fatalf("username fuzzy filter failed: %#v", usernameData)
	}
	if _, err := application.Actions.Invoke(ctx, operations.ActionAuditLogGet, json.RawMessage(`{"id":"a3"}`)); errorCode(err) != errs.CodeNotFound {
		t.Fatalf("cross-tenant detail must be opaque 404: %v", err)
	}
	var queryAuditCount int64
	if err := db.Model(&audit.Record{}).Where("tenant_id = ? AND action_id = ?", "t1", operations.ActionAuditLogList).Count(&queryAuditCount).Error; err != nil {
		t.Fatal(err)
	}
	if queryAuditCount != 2 {
		t.Fatalf("audit query must be audited, count=%d", queryAuditCount)
	}
}

func TestAuditLogQueryRejectsInvalidRangesAndPageSize(t *testing.T) {
	db := openOutboxTestDB(t)
	if err := db.AutoMigrate(&audit.Record{}, &model.User{}); err != nil {
		t.Fatal(err)
	}
	application := app.New(&config.Config{}, db, audit.NewGormRecorder(db), nil)
	if err := operations.Register(application, nil); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant, Permissions: []string{operations.ActionAuditLogList}})
	for _, input := range []string{
		`{"start_at":"invalid"}`,
		`{"start_at":"2026-01-01T00:00:00Z","end_at":"2026-03-01T00:00:00Z"}`,
		`{"page":1,"page_size":101}`,
	} {
		if _, err := application.Actions.Invoke(ctx, operations.ActionAuditLogList, json.RawMessage(input)); errorCode(err) != errs.CodeValidation {
			t.Fatalf("input %s must fail validation: %v", input, err)
		}
	}
}
