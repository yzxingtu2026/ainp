package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

type versionedCustomerProvider struct{ version atomic.Uint64 }

type customerAuthenticator struct{}

func (customerAuthenticator) Method() string { return "example-sso" }
func (customerAuthenticator) Capabilities() authentication.Capabilities {
	return authentication.Capabilities{}
}
func (customerAuthenticator) Begin(context.Context, authentication.BeginRequest) (*authentication.Challenge, error) {
	return nil, auth.ErrAuthorizationInvalid
}
func (customerAuthenticator) Complete(context.Context, authentication.CompleteRequest) (*authentication.VerifiedIdentity, error) {
	return &authentication.VerifiedIdentity{Provider: "example-sso", Subject: "customer-1", TenantID: "tenant-a", Principal: auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}, AuthVersion: 1}, nil
}

func (p *versionedCustomerProvider) Kind() auth.PrincipalKind { return "example.customer" }
func (p *versionedCustomerProvider) LoadAuthorization(_ context.Context, req auth.AuthorizationRequest) (*auth.AuthorizationSnapshot, error) {
	ref := req.PrincipalRef()
	version := p.version.Load()
	if req.TenantID != "tenant-a" || ref != (auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}) || req.AuthVersion != version {
		return nil, auth.ErrAuthorizationStale
	}
	return &auth.AuthorizationSnapshot{TenantID: req.TenantID, Principal: ref, PrincipalID: ref.ID, SessionID: req.SessionID, AuthVersion: version, Audience: req.Audience, ClientSurface: req.ClientSurface, Permissions: []string{"orders:read"}}, nil
}

func TestClientSurfaceSessionPolicies(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	root, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_session_policy_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if err = audit.NewGormRecorder(db).AutoMigrate(); err != nil {
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
	defaultSurface := auth.ClientSurface{ID: "example.default", Audience: "example:default", PathPrefix: "/default/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}}
	absoluteSurface := auth.ClientSurface{ID: "example.absolute", Audience: "example:absolute", PathPrefix: "/absolute/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}, AbsoluteTTL: 2 * time.Hour}
	idleSurface := auth.ClientSurface{ID: "example.idle", Audience: "example:idle", PathPrefix: "/idle/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}, IdleTTL: time.Minute}
	rejectSurface := auth.ClientSurface{ID: "example.reject", Audience: "example:reject", PathPrefix: "/reject/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}, MaxActiveSessions: 1, OverflowPolicy: auth.SessionOverflowReject}
	revokeSurface := auth.ClientSurface{ID: "example.revoke", Audience: "example:revoke", PathPrefix: "/revoke/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}, MaxActiveSessions: 1, OverflowPolicy: auth.SessionOverflowRevokeOldest}
	concurrentSurface := auth.ClientSurface{ID: "example.concurrent", Audience: "example:concurrent", PathPrefix: "/concurrent/", PrincipalKinds: []auth.PrincipalKind{"example.customer"}, Methods: []string{"example-sso"}, MaxActiveSessions: 2, OverflowPolicy: auth.SessionOverflowReject}
	for _, surface := range []auth.ClientSurface{defaultSurface, absoluteSurface, idleSurface, rejectSurface, revokeSurface, concurrentSurface} {
		if err = surfaces.Register(surface); err != nil {
			t.Fatal(err)
		}
	}
	tokens, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	sessions := system.NewSessionService(db, tokens, system.NewAuthService(db), 24*time.Hour)
	sessions.Configure(principals, surfaces)
	snapshot := &auth.AuthorizationSnapshot{TenantID: "tenant-a", Principal: auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}, PrincipalID: "customer-1", AuthVersion: 1}

	createdDefault, err := sessions.CreatePrincipal(context.Background(), snapshot, defaultSurface, "mobile", "default")
	if err != nil {
		t.Fatal(err)
	}
	createdAbsolute, err := sessions.CreatePrincipal(context.Background(), snapshot, absoluteSurface, "mobile", "absolute")
	if err != nil {
		t.Fatal(err)
	}
	var defaultRow, absoluteRow model.AuthSession
	if err = db.First(&defaultRow, "id = ?", createdDefault.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.First(&absoluteRow, "id = ?", createdAbsolute.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if remaining := time.Until(defaultRow.ExpiresAt); remaining < 23*time.Hour || remaining > 24*time.Hour+time.Minute {
		t.Fatalf("default lifetime=%s, want configured global TTL", remaining)
	}
	if remaining := time.Until(absoluteRow.ExpiresAt); remaining < time.Hour+59*time.Minute || remaining > 2*time.Hour+time.Minute {
		t.Fatalf("absolute lifetime=%s, want surface TTL", remaining)
	}

	idleTokens, err := sessions.CreatePrincipal(context.Background(), snapshot, idleSurface, "mobile", "idle")
	if err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-time.Minute)
	if err = db.Model(&model.AuthSession{}).Where("id = ?", idleTokens.SessionID).Update("last_seen_at", stale).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.Refresh(context.Background(), idleTokens.RefreshToken); appErrorKey(err) != "auth.session_idle_expired" {
		t.Fatalf("idle refresh error=%v key=%q", err, appErrorKey(err))
	}
	var idleRow model.AuthSession
	if err = db.First(&idleRow, "id = ?", idleTokens.SessionID).Error; err != nil || idleRow.RevokedAt == nil || idleRow.RevokedReason != "idle_expired" {
		t.Fatalf("idle row=%#v err=%v", idleRow, err)
	}
	var idleAuditCount int64
	if err = db.Model(&audit.Record{}).Where("resource_id = ? AND action_id = ? AND summary = ?", idleTokens.SessionID, "system.auth_session_policy_revoke", "idle_expired").Count(&idleAuditCount).Error; err != nil || idleAuditCount != 1 {
		t.Fatalf("idle audit count=%d err=%v", idleAuditCount, err)
	}
	protectedIdle, err := sessions.CreatePrincipal(context.Background(), snapshot, idleSurface, "mobile", "protected-idle")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&model.AuthSession{}).Where("id = ?", protectedIdle.SessionID).Update("last_seen_at", stale).Error; err != nil {
		t.Fatal(err)
	}
	protectedReq := auth.AuthorizationRequest{TenantID: "tenant-a", Principal: snapshot.Principal, PrincipalID: snapshot.PrincipalID, SessionID: protectedIdle.SessionID, Realm: auth.RealmTenant, AuthVersion: 1, Audience: idleSurface.Audience, ClientSurface: idleSurface.ID}
	if _, err = sessions.LoadAuthorization(context.Background(), protectedReq); appErrorKey(err) != "auth.session_idle_expired" {
		t.Fatalf("protected idle error=%v key=%q", err, appErrorKey(err))
	}
	activeIdle, err := sessions.CreatePrincipal(context.Background(), snapshot, idleSurface, "mobile", "protected-active")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC().Add(-30 * time.Second)
	if err = db.Model(&model.AuthSession{}).Where("id = ?", activeIdle.SessionID).Update("last_seen_at", before).Error; err != nil {
		t.Fatal(err)
	}
	protectedReq.SessionID = activeIdle.SessionID
	if _, err = sessions.LoadAuthorization(context.Background(), protectedReq); err != nil {
		t.Fatal(err)
	}
	var activeIdleRow model.AuthSession
	if err = db.First(&activeIdleRow, "id = ?", activeIdle.SessionID).Error; err != nil || !activeIdleRow.LastSeenAt.After(before) {
		t.Fatalf("protected request did not advance last_seen_at: row=%#v err=%v", activeIdleRow, err)
	}

	expiredAbsolute, err := sessions.CreatePrincipal(context.Background(), snapshot, absoluteSurface, "mobile", "expired-absolute")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&model.AuthSession{}).Where("id = ?", expiredAbsolute.SessionID).Update("expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.Refresh(context.Background(), expiredAbsolute.RefreshToken); appErrorKey(err) != "auth.session_absolute_expired" {
		t.Fatalf("absolute refresh error=%v key=%q", err, appErrorKey(err))
	}

	if _, err = sessions.CreatePrincipal(context.Background(), snapshot, rejectSurface, "mobile", "one"); err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.CreatePrincipal(context.Background(), snapshot, rejectSurface, "mobile", "two"); appErrorKey(err) != "auth.session_limit_reached" {
		t.Fatalf("reject overflow error=%v key=%q", err, appErrorKey(err))
	}

	oldest, err := sessions.CreatePrincipal(context.Background(), snapshot, revokeSurface, "mobile", "oldest")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.CreatePrincipal(context.Background(), snapshot, revokeSurface, "mobile", "newest"); err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.Refresh(context.Background(), oldest.RefreshToken); appErrorKey(err) != "auth.session_overflow_revoked" {
		t.Fatalf("oldest refresh error=%v key=%q", err, appErrorKey(err))
	}
	rotating, err := sessions.CreatePrincipal(context.Background(), snapshot, idleSurface, "mobile", "replay")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := sessions.Refresh(context.Background(), rotating.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.Refresh(context.Background(), rotating.RefreshToken); appErrorKey(err) != "auth.session_invalid" {
		t.Fatalf("policy surface replay error=%v key=%q", err, appErrorKey(err))
	}
	if _, err = sessions.Refresh(context.Background(), rotated.RefreshToken); appErrorKey(err) != "auth.session_invalid" {
		t.Fatalf("policy surface token family survived replay: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, createErr := sessions.CreatePrincipal(context.Background(), snapshot, concurrentSurface, "mobile", fmt.Sprintf("device-%d", i))
			results <- createErr
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for createErr := range results {
		if createErr == nil {
			successes++
		} else if appErrorKey(createErr) != "auth.session_limit_reached" {
			t.Fatalf("unexpected concurrent create error: %v", createErr)
		}
	}
	if successes != concurrentSurface.MaxActiveSessions {
		t.Fatalf("concurrent successes=%d want=%d", successes, concurrentSurface.MaxActiveSessions)
	}
	var activeCount int64
	if err = db.Model(&model.AuthSession{}).Where("tenant_id = ? AND principal_kind = ? AND principal_id = ? AND client_surface = ? AND revoked_at IS NULL", "tenant-a", "example.customer", "customer-1", concurrentSurface.ID).Count(&activeCount).Error; err != nil || activeCount != int64(concurrentSurface.MaxActiveSessions) {
		t.Fatalf("active concurrent sessions=%d err=%v", activeCount, err)
	}
}

func appErrorKey(err error) string {
	ae, ok := errs.AsAppError(err)
	if !ok {
		return ""
	}
	return ae.Key()
}

func TestSessionRefreshRotationReplayAndTenantScope(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_auth_" + uuid.NewString()[:8]
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
	for _, row := range []tenant.Tenant{{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled}, {ID: "tenant-b", Name: "B", Status: status.ActivationEnabled}} {
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	user := model.TenantAccount{ID: "user-a", TenantID: "tenant-a", Username: "admin", PasswordHash: "x", Status: status.ActivationEnabled, AuthVersion: 1}
	if err := testDB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	tokens, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	authService := system.NewAuthService(testDB)
	sessions := system.NewSessionService(testDB, tokens, authService)
	created, err := sessions.Create(context.Background(), &user, "web", "test")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.Parse(created.AccessToken)
	if err != nil || claims.SessionID != created.SessionID {
		t.Fatalf("claims/session mismatch: %#v %v", claims, err)
	}
	rotated, err := sessions.Refresh(context.Background(), created.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.RefreshToken == created.RefreshToken {
		t.Fatal("refresh token must rotate")
	}
	if _, err := sessions.Refresh(context.Background(), created.RefreshToken); err == nil {
		t.Fatal("replayed refresh must fail")
	}
	if _, err := sessions.Refresh(context.Background(), rotated.RefreshToken); err == nil {
		t.Fatal("replay must revoke the session family")
	}
	if _, err := authService.LoadAuthorization(context.Background(), auth.AuthorizationRequest{TenantID: "tenant-b", PrincipalID: user.ID, SessionID: created.SessionID, Realm: auth.RealmTenant, AuthVersion: 1}); err == nil {
		t.Fatal("cross-tenant session must fail")
	}
	userB := model.TenantAccount{ID: "user-b", TenantID: "tenant-b", Username: "admin-b", PasswordHash: "x", Status: status.ActivationEnabled, AuthVersion: 1}
	if err := testDB.Create(&userB).Error; err != nil {
		t.Fatal(err)
	}
	sessionA1, err := sessions.Create(context.Background(), &user, "web", "A1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Create(context.Background(), &user, "mobile", "A2"); err != nil {
		t.Fatal(err)
	}
	sessionB, err := sessions.Create(context.Background(), &userB, "web", "B1")
	if err != nil {
		t.Fatal(err)
	}
	_, secretA, _ := strings.Cut(sessionA1.RefreshToken, ".")
	if _, err := sessions.Refresh(context.Background(), sessionB.SessionID+"."+secretA); err == nil {
		t.Fatal("refresh secret from another tenant session must fail")
	}
	if _, err := sessions.Refresh(context.Background(), sessionB.RefreshToken); err != nil {
		t.Fatalf("forged refresh must not revoke target session: %v", err)
	}
	unified := system.NewUnifiedAuthService(testDB, nil, nil, nil, nil)
	ctxA := auth.WithContext(context.Background(), &auth.Context{
		Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", PrincipalID: "user-a", SessionID: sessionA1.SessionID,
	})
	listed, err := unified.ListSessions(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	views := listed.(map[string]any)["sessions"].([]system.SessionView)
	if len(views) != 2 {
		t.Fatalf("tenant session list=%#v", views)
	}
	if _, err := unified.RevokeSession(ctxA, sessionB.SessionID); appErrorCode(err) != errs.CodeNotFound {
		t.Fatalf("cross-tenant revoke must be opaque: %v", err)
	}
	if _, err := unified.RevokeSession(ctxA, sessionA1.SessionID); err != nil {
		t.Fatal(err)
	}
	revoked, err := unified.RevokeAll(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.(map[string]any)["revoked_count"] != int64(1) {
		t.Fatalf("revoke all=%#v", revoked)
	}
}

func TestPrincipalSessionRefreshVersionAndConcurrency(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	root, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_principal_auth_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	tokens, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	sessions := system.NewSessionService(db, tokens, system.NewAuthService(db))
	sessions.Configure(principals, surfaces)
	forged := &auth.AuthorizationSnapshot{TenantID: "tenant-a", Principal: auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}, PrincipalID: "customer-1", AuthVersion: 2}
	if _, err = sessions.CreatePrincipal(context.Background(), forged, surface, "mobile", "forged"); err == nil {
		t.Fatal("session service must revalidate principal snapshot before signing")
	}
	snapshot, err := principals.LoadAuthorization(context.Background(), auth.AuthorizationRequest{TenantID: "tenant-a", Principal: auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}, AuthVersion: 1, Audience: surface.Audience, ClientSurface: surface.ID})
	if err != nil {
		t.Fatal(err)
	}
	created, err := sessions.CreatePrincipal(context.Background(), snapshot, surface, "mobile", "phone")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.ParseFor(created.AccessToken, surface.Audience)
	if err != nil || claims.PrincipalKind != "example.customer" || claims.ClientSurface != surface.ID || claims.Subject != "customer-1" {
		t.Fatalf("custom claims=%#v err=%v", claims, err)
	}
	var row model.AuthSession
	if err = db.First(&row, "id = ?", created.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if row.AccountID != nil || row.PrincipalKind != "example.customer" || row.PrincipalID != "customer-1" {
		t.Fatalf("custom session leaked account semantics: %#v", row)
	}
	historySession, err := sessions.CreatePrincipal(context.Background(), snapshot, surface, "mobile", "history")
	if err != nil {
		t.Fatal(err)
	}
	historyRotated1, err := sessions.Refresh(context.Background(), historySession.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	historyRotated2, err := sessions.Refresh(context.Background(), historyRotated1.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.Refresh(context.Background(), historySession.RefreshToken); err == nil {
		t.Fatal("replaying any used refresh token must fail")
	}
	if _, err = sessions.Refresh(context.Background(), historyRotated2.RefreshToken); err == nil {
		t.Fatal("old refresh replay must revoke the complete token family")
	}

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, refreshErr := sessions.Refresh(context.Background(), created.RefreshToken)
			results <- refreshErr
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for refreshErr := range results {
		if refreshErr == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent refresh successes=%d, want 1", successes)
	}
	if err = db.First(&row, "id = ?", created.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	if row.RevokedAt == nil {
		t.Fatal("refresh replay must revoke token family")
	}

	created2, err := sessions.CreatePrincipal(context.Background(), snapshot, surface, "mobile", "phone-2")
	if err != nil {
		t.Fatal(err)
	}
	provider.version.Store(2)
	if _, err = sessions.Refresh(context.Background(), created2.RefreshToken); err == nil {
		t.Fatal("principal auth version change must invalidate refresh")
	}
}

func TestUnifiedAuthenticationCreatesCustomPrincipalSession(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	root, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_principal_login_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	methods := authentication.NewRegistry()
	if err = methods.Register(customerAuthenticator{}); err != nil {
		t.Fatal(err)
	}
	tokenService, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	authService := system.NewAuthService(db)
	sessions := system.NewSessionService(db, tokenService, authService)
	sessions.Configure(principals, surfaces)
	unified := system.NewUnifiedAuthService(db, methods, authService, sessions, nil)
	unified.Configure(principals, surfaces)
	ctx := auth.WithContext(context.Background(), &auth.Context{ClientSurface: surface.ID, Audience: surface.Audience})
	result, err := unified.Authenticate(ctx, system.AuthenticateRequest{Method: "example-sso", Client: surface.ID, Credentials: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	data := result.(map[string]any)
	claims, err := tokenService.ParseFor(data["access_token"].(string), surface.Audience)
	if err != nil || claims.PrincipalKind != "example.customer" || claims.Subject != "customer-1" {
		t.Fatalf("custom login claims=%#v err=%v", claims, err)
	}
	wrongPath := auth.WithContext(context.Background(), &auth.Context{ClientSurface: auth.TenantConsoleSurface, Audience: auth.TenantAudience})
	if _, err = unified.Authenticate(wrongPath, system.AuthenticateRequest{Method: "example-sso", Client: surface.ID, Credentials: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("credential exchange must match request path surface")
	}
}
