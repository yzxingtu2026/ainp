package tests

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

type fakeWebSocketHandshakeService struct {
	request auth.WebSocketHandshakeConsumeRequest
	result  *auth.Context
	err     error
}

func (f *fakeWebSocketHandshakeService) Issue(context.Context, string) (*auth.WebSocketHandshakeGrant, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeWebSocketHandshakeService) Consume(_ context.Context, request auth.WebSocketHandshakeConsumeRequest) (*auth.Context, error) {
	f.request = request
	return f.result, f.err
}

func TestWebSocketHandshakeTargetRegistryAndProtocol(t *testing.T) {
	surfaces := auth.NewClientSurfaceRegistry()
	if err := surfaces.Register(auth.ClientSurface{ID: "example.mobile", Audience: "example:mobile", PathPrefix: "/api/mobile/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}}); err != nil {
		t.Fatal(err)
	}
	targets := auth.NewWebSocketHandshakeTargetRegistry()
	if err := targets.Register(auth.WebSocketHandshakeTarget{ID: "orders.events", Path: "/api/mobile/events"}); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []auth.WebSocketHandshakeTarget{
		{ID: "orders.dynamic", Path: "/api/mobile/:id/events"},
		{ID: "orders.query", Path: "/api/mobile/events?token=x"},
		{ID: "orders.events", Path: "/api/mobile/other"},
		{ID: "orders.other", Path: "/api/mobile/events"},
	} {
		if err := targets.Register(invalid); err == nil {
			t.Fatalf("invalid target registered: %#v", invalid)
		}
	}
	if err := targets.Validate(surfaces); err != nil {
		t.Fatal(err)
	}
	targets.Seal()
	if err := targets.Register(auth.WebSocketHandshakeTarget{ID: "orders.late", Path: "/api/mobile/late"}); err == nil {
		t.Fatal("sealed registry must reject mutation")
	}

	protocols := auth.WebSocketHandshakeProtocols("grant-value")
	if len(protocols) != 2 || protocols[0] != auth.WebSocketHandshakeProtocol {
		t.Fatalf("protocols=%#v", protocols)
	}
	grant, err := auth.WebSocketHandshakeGrantFromHeader(strings.Join(protocols, ", "))
	if err != nil || grant != "grant-value" {
		t.Fatalf("grant=%q err=%v", grant, err)
	}
	for _, header := range []string{"ainp.ws.v1", "ainp.ws.grant.value", "ainp.ws.v1, ainp.ws.grant.a, ainp.ws.grant.b"} {
		if _, err := auth.WebSocketHandshakeGrantFromHeader(header); err == nil {
			t.Fatalf("invalid protocol header accepted: %q", header)
		}
	}
}

func TestAuthenticateWebSocketHandshakeRestoresRequestContext(t *testing.T) {
	service := &fakeWebSocketHandshakeService{result: &auth.Context{TenantID: "tenant-a", Principal: auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}, PrincipalID: "customer-1", Authenticated: true}}
	request := httptest.NewRequest(http.MethodGet, "/api/mobile/events", nil)
	request.Header.Set("Connection", "keep-alive, Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Protocol", strings.Join(auth.WebSocketHandshakeProtocols("grant-value"), ", "))
	authenticated, err := auth.AuthenticateWebSocketHandshake(request, service)
	if err != nil {
		t.Fatal(err)
	}
	ac := auth.MustFromContext(authenticated.Context())
	if !ac.Authenticated || ac.Principal.ID != "customer-1" || service.request.TargetPath != "/api/mobile/events" || service.request.Grant != "grant-value" {
		t.Fatalf("context=%#v request=%#v", ac, service.request)
	}

	notUpgrade := httptest.NewRequest(http.MethodGet, "/api/mobile/events", nil)
	notUpgrade.Header.Set("Sec-WebSocket-Protocol", strings.Join(auth.WebSocketHandshakeProtocols("grant-value"), ", "))
	if _, err := auth.AuthenticateWebSocketHandshake(notUpgrade, service); err == nil {
		t.Fatal("non-upgrade request must not consume a grant")
	}
}

func TestWebSocketHandshakeGrantLifecycleAndConcurrency(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	root, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_ws_auth_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = root.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&tenant.Tenant{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled}).Error; err != nil {
		t.Fatal(err)
	}

	provider := &versionedCustomerProvider{}
	provider.version.Store(1)
	principals := auth.NewPrincipalRegistry()
	if err = principals.Register(provider); err != nil {
		t.Fatal(err)
	}
	surfaces := auth.NewClientSurfaceRegistry()
	surface := auth.ClientSurface{ID: "example.mobile", Audience: "example:mobile", PathPrefix: "/api/mobile/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}}
	if err = surfaces.Register(surface); err != nil {
		t.Fatal(err)
	}
	if err = surfaces.Register(auth.BuiltinTenantConsoleSurface()); err != nil {
		t.Fatal(err)
	}
	targets := auth.NewWebSocketHandshakeTargetRegistry()
	for _, target := range []auth.WebSocketHandshakeTarget{{ID: "orders.events", Path: "/api/mobile/events"}, {ID: "orders.other", Path: "/api/mobile/other"}, {ID: "admin.events", Path: "/api/v1/events"}} {
		if err = targets.Register(target); err != nil {
			t.Fatal(err)
		}
	}
	tokens, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	sessions := system.NewSessionService(db, tokens, system.NewAuthService(db))
	sessions.Configure(principals, surfaces)
	principal := auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}
	snapshot, err := principals.LoadAuthorization(context.Background(), auth.AuthorizationRequest{TenantID: "tenant-a", Principal: principal, AuthVersion: 1, Audience: surface.Audience, ClientSurface: surface.ID})
	if err != nil {
		t.Fatal(err)
	}
	created, err := sessions.CreatePrincipal(context.Background(), snapshot, surface, "web", "browser")
	if err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", Principal: principal, PrincipalID: principal.ID, SessionID: created.SessionID, Realm: auth.RealmTenant, ClientSurface: surface.ID, Audience: surface.Audience, AuthVersion: 1, Authenticated: true})
	service := system.NewWebSocketHandshakeService(db, principals, surfaces, targets)

	issued, err := service.Issue(ctx, "/api/mobile/events")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Consume(context.Background(), auth.WebSocketHandshakeConsumeRequest{Grant: issued.Grant, TargetPath: "/api/mobile/other"}); err == nil {
		t.Fatal("cross-path grant must fail")
	}
	consumed, err := service.Consume(context.Background(), auth.WebSocketHandshakeConsumeRequest{Grant: issued.Grant, TargetPath: "/api/mobile/events"})
	if err != nil {
		t.Fatal(err)
	}
	if !consumed.Authenticated || consumed.Principal != principal || consumed.ClientSurface != surface.ID || consumed.Audience != surface.Audience || consumed.SessionID != created.SessionID || !consumed.HasPermission("orders:read") {
		t.Fatalf("consumed context=%#v", consumed)
	}
	if _, err = service.Consume(context.Background(), auth.WebSocketHandshakeConsumeRequest{Grant: issued.Grant, TargetPath: "/api/mobile/events"}); err == nil {
		t.Fatal("replayed grant must fail")
	}

	tampered, err := service.Issue(ctx, "/api/mobile/events")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Consume(context.Background(), auth.WebSocketHandshakeConsumeRequest{Grant: tampered.Grant + "x", TargetPath: "/api/mobile/events"}); err == nil {
		t.Fatal("tampered grant must fail")
	}
	if _, err = service.Issue(ctx, "/api/v1/events"); err == nil {
		t.Fatal("cross-surface target issue must fail")
	}

	expired, err := service.Issue(ctx, "/api/mobile/events")
	if err != nil {
		t.Fatal(err)
	}
	expiredID, _, _ := strings.Cut(expired.Grant, ".")
	if err = db.Model(&model.WebSocketHandshakeGrant{}).Where("id = ?", expiredID).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.Consume(context.Background(), auth.WebSocketHandshakeConsumeRequest{Grant: expired.Grant, TargetPath: "/api/mobile/events"}); err == nil {
		t.Fatal("expired grant must fail")
	}

	stale, err := service.Issue(ctx, "/api/mobile/events")
	if err != nil {
		t.Fatal(err)
	}
	provider.version.Store(2)
	if _, err = service.Consume(context.Background(), auth.WebSocketHandshakeConsumeRequest{Grant: stale.Grant, TargetPath: "/api/mobile/events"}); err == nil {
		t.Fatal("authorization drift must fail")
	}
	provider.version.Store(1)

	concurrent, err := service.Issue(ctx, "/api/mobile/events")
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, consumeErr := service.Consume(context.Background(), auth.WebSocketHandshakeConsumeRequest{Grant: concurrent.Grant, TargetPath: "/api/mobile/events"})
			results <- consumeErr
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	for consumeErr := range results {
		if consumeErr == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent handshake successes=%d, want 1", successes)
	}
}
