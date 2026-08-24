package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
	"github.com/yzxingtu2026/ainp/platform/migrations"
	"github.com/yzxingtu2026/ainp/platform/modules/system/notifications"
	notificationjobs "github.com/yzxingtu2026/ainp/platform/modules/system/notifications/jobs"
)

type notificationDeliveryContextStore struct {
	context *notificationjobs.DeliveryContext
}

func (s notificationDeliveryContextStore) LoadDelivery(context.Context, string) (*notificationjobs.DeliveryContext, error) {
	return s.context, nil
}

func (notificationDeliveryContextStore) UpdateDeliveryStatus(context.Context, string, notification.NotificationDeliveryStatus, string, string) error {
	return nil
}

func TestNotificationPrincipalContextMigration(t *testing.T) {
	migration, err := migrations.Find(63)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"principal_kind", "client_surface", "audience", "uq_notif_req_idem", "DROP INDEX"} {
		if !strings.Contains(migration.SQL, want) {
			t.Fatalf("migration 0063 missing %q", want)
		}
	}
}

func TestNotificationRequestPrincipalRef(t *testing.T) {
	row := notifications.NotificationRequest{ActorID: "customer-1", PrincipalKind: "example.customer"}
	if got := row.PrincipalRef(); got != (auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}) {
		t.Fatalf("principal ref = %#v", got)
	}
}

func TestIndependentWorkerRestoresPrincipalContext(t *testing.T) {
	body, err := os.ReadFile("../framework/asynqadapter/adapter.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"restoredIdentity(run)",
		"Principal:     principal",
		"ClientSurface: clientSurface",
		"Audience:      audience",
		`run.PrincipalKind == ""`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("independent Worker identity restoration missing %q", required)
		}
	}
}

func TestNotificationDeliveryRejectsIncompleteOrMismatchedJobIdentity(t *testing.T) {
	persisted := &notificationjobs.DeliveryContext{
		DeliveryID: "delivery-1", TenantID: "tenant-a",
		Principal:     auth.PrincipalRef{Kind: "example.customer", ID: "same-id"},
		ClientSurface: "example.customer-client", Audience: "example:customer",
	}
	handler := notificationjobs.NewNotificationDeliverHandler(notificationDeliveryContextStore{context: persisted}, nil, nil, nil, nil)
	payload := json.RawMessage(`{"delivery_id":"delivery-1"}`)

	for name, envelope := range map[string]job.Envelope{
		"missing identity": {TenantID: "tenant-a", ActorID: "same-id"},
		"different kind":   {TenantID: "tenant-a", ActorID: "same-id", Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "same-id"}, ClientSurface: "example.customer-client", Audience: "example:customer"},
		"different surface": {TenantID: "tenant-a", ActorID: "same-id", Principal: persisted.Principal,
			ClientSurface: "example.other-client", Audience: "example:customer"},
		"different audience": {TenantID: "tenant-a", ActorID: "same-id", Principal: persisted.Principal,
			ClientSurface: "example.customer-client", Audience: "example:other"},
		"different tenant": {TenantID: "tenant-b", ActorID: "same-id", Principal: persisted.Principal,
			ClientSurface: "example.customer-client", Audience: "example:customer"},
	} {
		t.Run(name, func(t *testing.T) {
			err := handler.Handle(context.Background(), envelope, payload, nil)
			if err == nil || !job.IsPermanent(err) {
				t.Fatalf("identity mismatch was not rejected permanently: %v", err)
			}
		})
	}
}

func TestNotificationOrchestratorRejectsCallerSelectedPrincipalContext(t *testing.T) {
	trusted := &auth.Context{Authenticated: true, TenantID: "tenant-a", PrincipalID: "same-id", Principal: auth.PrincipalRef{Kind: "example.customer", ID: "same-id"}, ClientSurface: "example.customer-client", Audience: "example:customer"}
	ctx := auth.WithContext(context.Background(), trusted)
	orchestrator := notification.NewOrchestrator(nil, nil, nil, nil, nil, nil)

	for name, input := range map[string]notification.NotificationInput{
		"invalid kind":   {TenantID: "tenant-a", ActorID: "same-id", Principal: auth.PrincipalRef{Kind: "INVALID", ID: "same-id"}, ClientSurface: trusted.ClientSurface, Audience: trusted.Audience},
		"missing id":     {TenantID: "tenant-a", ActorID: "same-id", Principal: auth.PrincipalRef{Kind: "example.customer"}, ClientSurface: trusted.ClientSurface, Audience: trusted.Audience},
		"different kind": {TenantID: "tenant-a", ActorID: "same-id", Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "same-id"}, ClientSurface: trusted.ClientSurface, Audience: trusted.Audience},
		"different surface": {TenantID: "tenant-a", ActorID: "same-id", Principal: trusted.Principal,
			ClientSurface: "example.other-client", Audience: trusted.Audience},
		"different audience": {TenantID: "tenant-a", ActorID: "same-id", Principal: trusted.Principal,
			ClientSurface: trusted.ClientSurface, Audience: "example:other"},
		"different tenant": {TenantID: "tenant-b", ActorID: "same-id", Principal: trusted.Principal,
			ClientSurface: trusted.ClientSurface, Audience: trusted.Audience},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := orchestrator.Process(ctx, input); err == nil {
				t.Fatal("caller-selected notification identity was accepted")
			}
		})
	}
}

func TestNotificationOrchestratorRejectsExplicitInvalidTrustedPrincipal(t *testing.T) {
	trusted := &auth.Context{
		Authenticated: true,
		TenantID:      "tenant-a",
		PrincipalID:   "same-id",
		Principal:     auth.PrincipalRef{Kind: "INVALID", ID: "same-id"},
		ClientSurface: "example.customer-client",
		Audience:      "example:customer",
	}
	orchestrator := notification.NewOrchestrator(nil, nil, nil, nil, nil, nil)
	input := notification.NotificationInput{TenantID: "tenant-a", IdempotencyKey: "same-key"}

	if _, _, err := orchestrator.Process(auth.WithContext(context.Background(), trusted), input); err == nil {
		t.Fatal("explicit invalid trusted Principal was treated as a legacy Account")
	}
}

func TestNotificationPrincipalContextMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_notification_principal_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE notification_requests (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, definition_key varchar(256) NOT NULL, actor_id varchar(64) NOT NULL DEFAULT '', idempotency_key varchar(256) NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now())`,
		`CREATE UNIQUE INDEX uq_notification_requests_idem ON notification_requests (tenant_id, definition_key, idempotency_key) WHERE idempotency_key != ''`,
		`INSERT INTO notification_requests(id,tenant_id,definition_key,actor_id,idempotency_key) VALUES ('legacy','tenant-a','demo.ready','same-id','same-key')`,
	} {
		if err = db.Exec(statement).Error; err != nil {
			t.Fatalf("prepare pre-0063 schema: %v\n%s", err, statement)
		}
	}
	applyTestMigration(t, db, "0063_notification_principal_context.sql")
	applyTestMigration(t, db, "0063_notification_principal_context.sql")
	if err = db.Exec(`INSERT INTO notification_requests(id,tenant_id,definition_key,actor_id,principal_kind,client_surface,audience,idempotency_key) VALUES ('custom','tenant-a','demo.ready','same-id','example.customer','example.customer-client','example:customer','same-key')`).Error; err != nil {
		t.Fatalf("different Principal context must have an independent notification idempotency key: %v", err)
	}
	if err = db.Exec(`INSERT INTO notification_requests(id,tenant_id,definition_key,actor_id,principal_kind,client_surface,audience,idempotency_key) VALUES ('duplicate','tenant-a','demo.ready','same-id','example.customer','example.customer-client','example:customer','same-key')`).Error; err == nil {
		t.Fatal("same Principal context reused a notification idempotency key")
	}
	var count int64
	if err = db.Raw(`SELECT count(*) FROM notification_requests WHERE id='legacy' AND principal_kind='ainp.account' AND client_surface='ainp.tenant-console' AND audience='ainp:tenant-console'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy notification request was not backfilled: count=%d err=%v", count, err)
	}
}

func TestNotificationRequestPersistsAndIsolatesPrincipalContextPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_notification_request_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	repo := notifications.NewNotificationRepository(db)
	if err = repo.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	definitions := notification.NewDefinitionRegistry()
	if err = definitions.Register(notification.Definition{
		Key: "demo.ready", Category: "demo", Surface: notification.SurfaceTenant,
		NameKey: "demo.ready", Priority: notification.PriorityNormal,
		AllowedChannels: []notification.Channel{notification.ChannelInApp}, DefaultChannels: []notification.Channel{notification.ChannelInApp},
		Templates: &notification.TemplateDefinition{Version: 1, Variants: map[string]notification.ChannelTemplate{"in_app:en": {Channel: notification.ChannelInApp, Locale: "en", InApp: &notification.InAppTemplate{Title: "Ready"}}}},
	}); err != nil {
		t.Fatal(err)
	}
	renderer := notification.NewRenderer()
	service := notifications.NewNotificationService(db, repo, definitions, renderer, notification.NewTemplateResolver(renderer, nil), nil, nil, nil)

	customAuth := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", PrincipalID: "same-id", Principal: auth.PrincipalRef{Kind: "example.customer", ID: "same-id"}, ClientSurface: "example.customer-client", Audience: "example:customer"}
	customCtx := auth.WithContext(context.Background(), customAuth)
	input := notification.NotificationInput{DefinitionKey: "demo.ready", TenantID: "tenant-a", IdempotencyKey: "same-key"}
	customID, _, err := service.Orchestrator().Process(customCtx, input)
	if err != nil {
		t.Fatal(err)
	}
	replayedID, _, err := service.Orchestrator().Process(customCtx, input)
	if err != nil || replayedID != customID {
		t.Fatalf("same Principal notification did not replay idempotently: first=%s replay=%s err=%v", customID, replayedID, err)
	}
	row, err := repo.GetRequest(customCtx, customID)
	if err != nil || row.PrincipalRef() != customAuth.Principal || row.ClientSurface != customAuth.ClientSurface || row.Audience != customAuth.Audience {
		t.Fatalf("notification Principal context was not persisted: row=%#v err=%v", row, err)
	}

	accountAuth := &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", PrincipalID: "same-id", Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "same-id"}, ClientSurface: auth.TenantConsoleSurface, Audience: auth.TenantAudience}
	accountID, _, err := service.Orchestrator().Process(auth.WithContext(context.Background(), accountAuth), input)
	if err != nil || accountID == customID {
		t.Fatalf("same ID from another Principal context shared notification idempotency: custom=%s account=%s err=%v", customID, accountID, err)
	}
}
