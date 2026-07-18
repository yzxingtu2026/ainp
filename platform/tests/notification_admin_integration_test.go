package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/jobs"
	"gorm.io/gorm"
)

type recordingJobEnqueuer struct {
	inputs []job.EnqueueInput
	err    error
}

func (e *recordingJobEnqueuer) Enqueue(_ context.Context, input job.EnqueueInput) (*job.Run, error) {
	if e.err != nil {
		return nil, e.err
	}
	e.inputs = append(e.inputs, input)
	return &job.Run{ID: uuid.NewString()}, nil
}

type failingOutboxWriter struct{}

func (failingOutboxWriter) Append(context.Context, *gorm.DB, outbox.AppendInput) (outbox.Envelope, error) {
	return outbox.Envelope{}, errors.New("forced outbox failure")
}

func TestNotificationDeliveryAdminFiltersRetryAndTenantIsolation(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_notification_admin_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := system.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}
	repo := system.NewNotificationRepository(testDB)
	if err := repo.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	for _, row := range []tenant.Tenant{
		{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled},
		{ID: "tenant-b", Name: "B", Status: status.ActivationEnabled},
	} {
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	deliveries := []system.NotificationDelivery{
		{ID: "a-failed-email", TenantID: "tenant-a", RequestID: "req-a", RecipientID: "user-a", Channel: "email", Status: "failed"},
		{ID: "a-dead-inapp", TenantID: "tenant-a", RequestID: "req-a", RecipientID: "user-a", Channel: "in_app", Status: "dead"},
		{ID: "a-delivered", TenantID: "tenant-a", RequestID: "req-a", RecipientID: "user-a", Channel: "email", Status: "delivered"},
		{ID: "b-failed-email", TenantID: "tenant-b", RequestID: "req-b", RecipientID: "user-b", Channel: "email", Status: "failed"},
	}
	for i := range deliveries {
		if err := testDB.Create(&deliveries[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	ctx := auth.WithContext(context.Background(), &auth.Context{
		TenantID: "tenant-a", UserID: "admin-a", Realm: auth.RealmTenant, Authenticated: true,
	})
	enqueuer := &recordingJobEnqueuer{}
	service := system.NewNotificationDeliveryService(repo, enqueuer)

	defaultResult, err := service.List(ctx, json.RawMessage(`{"page":1,"page_size":20}`))
	if err != nil {
		t.Fatal(err)
	}
	if total := defaultResult.(pagination.Result[map[string]any]).Total; total != 3 {
		t.Fatalf("default list total = %d, want all tenant delivery statuses", total)
	}
	filtered, err := service.List(ctx, json.RawMessage(`{"page":1,"page_size":20,"status":"failed","channel":"email"}`))
	if err != nil {
		t.Fatal(err)
	}
	filteredPage := filtered.(pagination.Result[map[string]any])
	if total := filteredPage.Total; total != 1 {
		t.Fatalf("filtered total = %d, want 1", total)
	}
	items := filteredPage.Items
	if items[0]["id"] != "a-failed-email" {
		t.Fatalf("tenant filter returned %#v", items)
	}
	if _, err := service.Retry(ctx, json.RawMessage(`{"id":"a-delivered"}`)); err == nil {
		t.Fatal("delivered notification must not be retryable")
	}
	if len(enqueuer.inputs) != 0 {
		t.Fatal("non-retryable delivery was enqueued")
	}

	if _, err := service.Retry(ctx, json.RawMessage(`{"id":"a-failed-email"}`)); err != nil {
		t.Fatal(err)
	}
	if len(enqueuer.inputs) != 1 || enqueuer.inputs[0].Type != jobs.NotificationDeliverJobType {
		t.Fatalf("retry enqueue inputs = %#v", enqueuer.inputs)
	}
	if !strings.HasPrefix(enqueuer.inputs[0].IdempotencyKey, "notification-retry:a-failed-email:") {
		t.Fatalf("retry idempotency key = %q", enqueuer.inputs[0].IdempotencyKey)
	}
	var retried system.NotificationDelivery
	if err := testDB.First(&retried, "id = ?", "a-failed-email").Error; err != nil {
		t.Fatal(err)
	}
	if retried.Status != "pending" {
		t.Fatalf("retried status = %q", retried.Status)
	}

	enqueuer.err = errors.New("queue unavailable")
	if _, err := service.Retry(ctx, json.RawMessage(`{"id":"a-dead-inapp"}`)); err == nil {
		t.Fatal("retry should fail when enqueue fails")
	}
	var notRetried system.NotificationDelivery
	if err := testDB.First(&notRetried, "id = ?", "a-dead-inapp").Error; err != nil {
		t.Fatal(err)
	}
	if notRetried.Status != "dead" {
		t.Fatalf("failed enqueue changed delivery status to %q", notRetried.Status)
	}
}
